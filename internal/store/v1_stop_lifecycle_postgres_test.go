package store

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
)

func TestV1StopLifecycleConcurrentDeliveryAndCompletion(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for the disposable PostgreSQL STOP lifecycle concurrency regression")
	}
	ctx := context.Background()
	s, err := NewPostgresStore(config.Config{DatabaseURL: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.db.Close() })

	var completionKeysExist bool
	err = s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='v1_transaction_completion_fact_keys')`).Scan(&completionKeysExist)
	if err != nil {
		t.Fatal(err)
	}
	if !completionKeysExist {
		t.Fatal("TEST_DATABASE_URL is missing migration 017_enforce_v1_transaction_completion_fact_uniqueness.sql")
	}

	for _, deliveryFirst := range []bool{true, false} {
		name := "charger_completion_first"
		if deliveryFirst {
			name = "remote_stop_acceptance_first"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newV1StopLifecycleFixture(t, s)
			start := make(chan struct{})
			var ready sync.WaitGroup
			ready.Add(2)
			errs := make(chan error, 2)
			deliver := func() {
				ready.Done()
				<-start
				_, err := s.MarkV1StopDelivery(ctx, fixture.transaction.HALTransactionID, "Accepted", "Accepted", "")
				errs <- err
			}
			complete := func() {
				ready.Done()
				<-start
				_, err := s.CompleteV1Transaction(ctx, fixture.transaction.HALTransactionID, fixture.meterStopWh, "Remote", fixture.completedAt, fixture.completedAt)
				errs <- err
			}
			if deliveryFirst {
				go deliver()
				go complete()
			} else {
				go complete()
				go deliver()
			}
			ready.Wait()
			close(start)
			for range 2 {
				if err := <-errs; err != nil {
					t.Fatal(err)
				}
			}

			completed, err := s.GetV1Transaction(ctx, fixture.transaction.HALTransactionID)
			if err != nil || completed.CompletedAt == nil || !completed.CompletedAt.Equal(fixture.completedAt) || completed.StopState != "COMPLETED" {
				t.Fatalf("transaction=%#v err=%v", completed, err)
			}
			workflow, err := s.GetV1StopWorkflow(ctx, fixture.transaction.HALTransactionID)
			if err != nil || workflow.State != "COMPLETED" || workflow.CompletedAt == nil || !workflow.CompletedAt.Equal(fixture.completedAt) {
				t.Fatalf("workflow=%#v err=%v", workflow, err)
			}
			var completionFacts int
			if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM v1_fact_outbox WHERE fact_type='transaction.completed' AND aggregate_key=$1`, fixture.transaction.HALTransactionID).Scan(&completionFacts); err != nil {
				t.Fatal(err)
			}
			if completionFacts != 1 {
				t.Fatalf("completion facts=%d, want exactly one", completionFacts)
			}
			var reservedFactID string
			if err := s.db.QueryRowContext(ctx, `SELECT fact_id::text FROM v1_transaction_completion_fact_keys WHERE aggregate_key=$1`, fixture.transaction.HALTransactionID).Scan(&reservedFactID); err != nil || reservedFactID == "" {
				t.Fatalf("completion fact key=%q err=%v", reservedFactID, err)
			}
			if _, err := s.CompleteV1Transaction(ctx, fixture.transaction.HALTransactionID, fixture.meterStopWh, "Remote", fixture.completedAt, fixture.completedAt); err != nil {
				t.Fatalf("exact completion retry=%v", err)
			}
			if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM v1_fact_outbox WHERE fact_type='transaction.completed' AND aggregate_key=$1`, fixture.transaction.HALTransactionID).Scan(&completionFacts); err != nil || completionFacts != 1 {
				t.Fatalf("completion retry facts=%d err=%v", completionFacts, err)
			}
		})
	}
}

type v1StopLifecycleFixture struct {
	transaction *V1Transaction
	meterStopWh int64
	completedAt time.Time
}

func newV1StopLifecycleFixture(t *testing.T, s *PostgresStore) v1StopLifecycleFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	mapping := V1MappingInput{
		CPOID:               NewUUIDString(),
		CMSChargerID:        NewUUIDString(),
		ChargerOCPPIdentity: "CP-STOP-RACE-" + NewUUIDString()[:8],
		Enabled:             true,
		CorrelationID:       "stop-lifecycle-race",
		RequestDigest:       "stop-lifecycle-race-mapping",
		Connectors:          []V1ConnectorMappingInput{{CMSConnectorID: NewUUIDString(), OCPPConnectorNumber: 1}},
	}
	if _, existed, err := s.SyncV1Mapping(ctx, mapping); err != nil || existed {
		t.Fatalf("mapping existed=%v err=%v", existed, err)
	}
	startInput := V1StartCommandInput{
		CMSCommandID:        NewUUIDString(),
		RequestDigest:       "stop-lifecycle-race-start",
		CPOID:               mapping.CPOID,
		CustomerID:          NewUUIDString(),
		CorrelationID:       "stop-lifecycle-race",
		CMSStartIntentID:    NewUUIDString(),
		CMSChargerID:        mapping.CMSChargerID,
		CMSConnectorID:      mapping.Connectors[0].CMSConnectorID,
		ChargerOCPPIdentity: mapping.ChargerOCPPIdentity,
		OCPPConnectorNumber: 1,
		IDTag:               "appv1_" + NewUUIDString()[:12],
		CredentialExpiresAt: now.Add(time.Minute),
		CommandExpiresAt:    now.Add(2 * time.Minute),
		MaxDurationSeconds:  int64ptr(600),
		DurationLimitSource: "CUSTOMER_TIME",
		EnergyLimitSource:   "NONE",
		LimitType:           "TIME",
	}
	startCommand, duplicate, err := s.CreateV1StartCommand(ctx, startInput)
	if err != nil || duplicate {
		t.Fatalf("start command=%#v duplicate=%v err=%v", startCommand, duplicate, err)
	}
	transaction, duplicate, err := s.MaterializeV1Start(ctx, V1StartMaterialization{
		ChargerOCPPIdentity: mapping.ChargerOCPPIdentity,
		OCPPConnectorNumber: 1,
		IDTag:               startInput.IDTag,
		MeterStartWh:        1000,
		ActualStartedAt:     now,
		ObservedAt:          now,
		OCPPTransactionID:   RandomTransactionID(),
	})
	if err != nil || duplicate {
		t.Fatalf("materialize=%#v duplicate=%v err=%v", transaction, duplicate, err)
	}
	stopInput := V1StopCommandInput{
		CMSCommandID:           NewUUIDString(),
		RequestDigest:          "stop-lifecycle-race-stop",
		CPOID:                  mapping.CPOID,
		CMSChargingSessionID:   NewUUIDString(),
		CMSChargerID:           mapping.CMSChargerID,
		CMSConnectorID:         mapping.Connectors[0].CMSConnectorID,
		ChargerOCPPIdentity:    mapping.ChargerOCPPIdentity,
		OCPPConnectorNumber:    1,
		HALTransactionID:       transaction.HALTransactionID,
		OCPPTransactionID:      transaction.OCPPTransactionID,
		RequestedStopInitiator: "CPO",
		RequestedStopReason:    "cpo_requested",
		CommandExpiresAt:       now.Add(time.Minute),
	}
	stopCommand, duplicate, err := s.CreateV1StopCommand(ctx, stopInput)
	if err != nil || duplicate {
		t.Fatalf("stop command=%#v duplicate=%v err=%v", stopCommand, duplicate, err)
	}
	t.Cleanup(func() {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_transaction_completion_fact_keys WHERE aggregate_key=$1`, transaction.HALTransactionID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_fact_outbox WHERE aggregate_key IN ($1,$2,$3)`, transaction.HALTransactionID, startCommand.HALCommandID, stopCommand.HALCommandID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_stop_workflows WHERE hal_transaction_id=$1`, transaction.HALTransactionID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_transactions WHERE hal_transaction_id=$1`, transaction.HALTransactionID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_start_credentials WHERE cms_start_intent_id=$1::uuid`, startInput.CMSStartIntentID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_remote_commands WHERE cms_command_id IN ($1::uuid,$2::uuid)`, startInput.CMSCommandID, stopInput.CMSCommandID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_connector_runtime WHERE charger_ocpp_identity=$1`, mapping.ChargerOCPPIdentity)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_charger_runtime WHERE charger_ocpp_identity=$1`, mapping.ChargerOCPPIdentity)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_mapping_audit WHERE cms_charger_id=$1::uuid`, mapping.CMSChargerID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_connector_mappings WHERE cms_charger_id=$1::uuid`, mapping.CMSChargerID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_charger_mappings WHERE cms_charger_id=$1::uuid`, mapping.CMSChargerID)
	})
	return v1StopLifecycleFixture{transaction: transaction, meterStopWh: 1100, completedAt: now.Add(time.Second)}
}
