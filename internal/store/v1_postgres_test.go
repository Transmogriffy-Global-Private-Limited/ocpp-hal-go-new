package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
)

func TestV1PostgresStoreDurabilityAndRuntime(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL v1 integration test")
	}
	ctx := context.Background()
	s, err := NewPostgresStore(config.Config{DatabaseURL: dsn})
	if err != nil {
		t.Fatal(err)
	}

	input := V1MappingInput{CPOID: NewUUIDString(), CMSChargerID: NewUUIDString(), ChargerOCPPIdentity: "CP-V1-PG-" + NewUUIDString()[:8], Enabled: true, CorrelationID: "test-correlation", RequestDigest: "mapping-digest", Connectors: []V1ConnectorMappingInput{{CMSConnectorID: NewUUIDString(), OCPPConnectorNumber: 1}}}
	mapping, existed, err := s.SyncV1Mapping(ctx, input)
	if err != nil || existed || len(mapping.Connectors) != 1 {
		t.Fatalf("sync mapping=%#v existed=%v err=%v", mapping, existed, err)
	}
	if _, existed, err = s.SyncV1Mapping(ctx, input); err != nil || !existed {
		t.Fatalf("repeat mapping existed=%v err=%v", existed, err)
	}
	conflict := input
	conflict.ChargerOCPPIdentity = "CP-V1-PG-OTHER"
	if _, _, err = s.SyncV1Mapping(ctx, conflict); err != ErrV1MappingConflict {
		t.Fatalf("mapping conflict=%v", err)
	}
	if err = s.ValidateV1Mapping(ctx, input.CPOID, input.CMSChargerID, input.Connectors[0].CMSConnectorID, input.ChargerOCPPIdentity, 1); err != nil {
		t.Fatal(err)
	}
	if err = s.ValidateV1ChargerAdmission(ctx, input.ChargerOCPPIdentity, ""); err != nil {
		t.Fatal(err)
	}
	if err = s.ValidateV1ChargerAdmission(ctx, "CP-V1-UNKNOWN-"+NewUUIDString()[:8], ""); err != ErrV1MappingNotFound {
		t.Fatalf("unknown charger admission=%v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	energy, duration := int64(9000), int64(3600)
	commandInput := V1StartCommandInput{CMSCommandID: NewUUIDString(), RequestDigest: "command-digest", CPOID: input.CPOID, CustomerID: NewUUIDString(), CorrelationID: "test-correlation", CMSStartIntentID: NewUUIDString(), CMSChargerID: input.CMSChargerID, CMSConnectorID: input.Connectors[0].CMSConnectorID, ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: 1, IDTag: "appv1_" + NewUUIDString()[:12], CredentialExpiresAt: now.Add(time.Minute), CommandExpiresAt: now.Add(2 * time.Minute), EnergyLimitWh: &energy, MaxDurationSeconds: &duration}
	type commandResult struct {
		command *V1RemoteCommand
		existed bool
		err     error
	}
	results := make(chan commandResult, 2)
	for range 2 {
		go func() {
			command, existed, err := s.CreateV1StartCommand(ctx, commandInput)
			results <- commandResult{command, existed, err}
		}()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.command.HALCommandID != second.command.HALCommandID || first.existed == second.existed {
		t.Fatalf("concurrent commands first=%#v second=%#v", first, second)
	}
	command := first.command
	if _, claimed, err := s.ClaimV1StartDelivery(ctx, commandInput.CMSCommandID); err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	if _, claimed, err := s.ClaimV1StartDelivery(ctx, commandInput.CMSCommandID); err != nil || claimed {
		t.Fatalf("repeat claim=%v err=%v", claimed, err)
	}
	replayed, existed, err := s.CreateV1StartCommand(ctx, commandInput)
	if err != nil || !existed || replayed.HALCommandID != command.HALCommandID {
		t.Fatalf("replay=%#v existed=%v err=%v", replayed, existed, err)
	}
	changed := commandInput
	changed.RequestDigest = "different"
	if _, _, err = s.CreateV1StartCommand(ctx, changed); err != ErrV1IdempotencyConflict {
		t.Fatalf("command conflict=%v", err)
	}
	if err = s.AuthorizeV1Credential(ctx, input.ChargerOCPPIdentity, commandInput.IDTag, now); err != nil {
		t.Fatal(err)
	}
	if err = s.AuthorizeV1Credential(ctx, "CP-WRONG", commandInput.IDTag, now); err != ErrV1CredentialRejected {
		t.Fatalf("wrong charger authorize=%v", err)
	}
	credential, err := s.GetV1Credential(ctx, commandInput.IDTag)
	if err != nil || credential.ChargerOCPPIdentity != input.ChargerOCPPIdentity || credential.OCPPConnectorNumber != 1 {
		t.Fatalf("credential=%#v err=%v", credential, err)
	}
	ocppTransactionID := RandomTransactionID()
	tx, duplicate, err := s.MaterializeV1Start(ctx, V1StartMaterialization{ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: 1, IDTag: commandInput.IDTag, MeterStartWh: 12345, ActualStartedAt: now, ObservedAt: now, OCPPTransactionID: ocppTransactionID})
	if err != nil || duplicate {
		t.Fatalf("materialize=%#v duplicate=%v err=%v", tx, duplicate, err)
	}
	retransmit, duplicate, err := s.MaterializeV1Start(ctx, V1StartMaterialization{ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: 1, IDTag: commandInput.IDTag, MeterStartWh: 12345, ActualStartedAt: now, ObservedAt: now, OCPPTransactionID: RandomTransactionID()})
	if err != nil || !duplicate || retransmit.OCPPTransactionID != ocppTransactionID {
		t.Fatalf("retransmit=%#v duplicate=%v err=%v", retransmit, duplicate, err)
	}
	if tx.StopDeadlineAt == nil || !tx.StopDeadlineAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("deadline=%v", tx.StopDeadlineAt)
	}
	updated, accepted, err := s.UpdateV1MeterForOCPP(ctx, input.ChargerOCPPIdentity, ocppTransactionID, 21345, now.Add(time.Second))
	if err != nil || !accepted || updated.MeterSequence != 1 || updated.LatestMeterWh == nil || *updated.LatestMeterWh != 21345 {
		t.Fatalf("accepted meter=%#v accepted=%v err=%v", updated, accepted, err)
	}
	regressive, accepted, err := s.UpdateV1MeterForOCPP(ctx, input.ChargerOCPPIdentity, ocppTransactionID, 20000, now.Add(2*time.Second))
	if err != nil || accepted || regressive.MeterSequence != 1 || *regressive.LatestMeterWh != 21345 {
		t.Fatalf("regressive meter=%#v accepted=%v err=%v", regressive, accepted, err)
	}
	quantized, accepted, err := s.UpdateV1MeterForOCPP(ctx, input.ChargerOCPPIdentity, ocppTransactionID, 21344, now.Add(2*time.Second))
	if err != nil || accepted || quantized.MeterSequence != 1 || quantized.LatestMeterWh == nil || *quantized.LatestMeterWh != 21345 || quantized.MeterQuantizationAnomalyCount != 1 {
		t.Fatalf("one Wh periodic rollback=%#v accepted=%v err=%v", quantized, accepted, err)
	}
	workflow, err := s.GetV1StopWorkflow(ctx, tx.HALTransactionID)
	if err != nil || workflow.RequestedStopInitiator != "ENERGY_LIMIT" || workflow.State != "PERSISTED" {
		t.Fatalf("energy workflow=%#v err=%v", workflow, err)
	}
	workflow, created, err := s.EnsureV1StopWorkflow(ctx, tx.HALTransactionID, "CPO", "cpo_requested")
	if err != nil || created || workflow.RequestedStopInitiator != "ENERGY_LIMIT" {
		t.Fatalf("racing workflow=%#v created=%v err=%v", workflow, created, err)
	}
	stopInput := V1StopCommandInput{CMSCommandID: NewUUIDString(), RequestDigest: "stop-digest", CPOID: input.CPOID, CMSChargingSessionID: NewUUIDString(), CMSChargerID: input.CMSChargerID, CMSConnectorID: input.Connectors[0].CMSConnectorID, ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: 1, HALTransactionID: tx.HALTransactionID, OCPPTransactionID: ocppTransactionID, RequestedStopInitiator: "CPO", RequestedStopReason: "cpo_requested", CommandExpiresAt: now.Add(time.Minute)}
	if stop, duplicate, err := s.CreateV1StopCommand(ctx, stopInput); err != nil || duplicate || stop.HALTransactionID != tx.HALTransactionID {
		t.Fatalf("stop command=%#v duplicate=%v err=%v", stop, duplicate, err)
	}
	if stop, duplicate, err := s.CreateV1StopCommand(ctx, stopInput); err != nil || !duplicate || stop.HALTransactionID != tx.HALTransactionID {
		t.Fatalf("duplicate stop command=%#v duplicate=%v err=%v", stop, duplicate, err)
	}
	completed, err := s.CompleteV1Transaction(ctx, tx.HALTransactionID, 21344, "Remote", now.Add(3*time.Second), now.Add(3*time.Second))
	if err != nil || completed.CompletedAt == nil || completed.MeterStopWh == nil || *completed.MeterStopWh != 21345 || completed.RawMeterStopWh == nil || *completed.RawMeterStopWh != 21344 || completed.MeterStopAdjustmentWh == nil || *completed.MeterStopAdjustmentWh != 1 || completed.MeterStopEvidence != string(v1MeterEvidenceQuantizationNormalized) || completed.OCPPStopReason != "Remote" || completed.RequestedStopInitiator != "ENERGY_LIMIT" {
		t.Fatalf("completion=%#v err=%v", completed, err)
	}
	facts, err := s.ClaimV1Facts(ctx, now.Add(time.Minute), 20)
	if err != nil || len(facts) < 3 {
		t.Fatalf("facts=%d err=%v", len(facts), err)
	}
	var completionPayload map[string]any
	for _, fact := range facts {
		if fact.FactType == "transaction.completed" {
			if err := json.Unmarshal(fact.Payload, &completionPayload); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if completionPayload["meter_stop_wh"] != float64(21345) || completionPayload["raw_meter_stop_wh"] != float64(21344) || completionPayload["meter_stop_adjustment_wh"] != float64(1) || completionPayload["meter_stop_evidence"] != string(v1MeterEvidenceQuantizationNormalized) {
		t.Fatalf("completion fact payload=%#v", completionPayload)
	}
	command, err = s.GetV1Command(ctx, commandInput.CMSCommandID)
	if err != nil || command.State != "MATERIALIZED" {
		t.Fatalf("materialized command=%#v err=%v", command, err)
	}
	if _, claimed, err := s.ClaimV1StartDelivery(ctx, commandInput.CMSCommandID); err != nil || claimed {
		t.Fatalf("materialized command redispatch claim=%v err=%v", claimed, err)
	}
	recoveryInput := commandInput
	recoveryInput.CMSCommandID, recoveryInput.CMSStartIntentID, recoveryInput.IDTag = NewUUIDString(), NewUUIDString(), "appv1_"+NewUUIDString()[:12]
	recoveryInput.RequestDigest = "recovery-digest"
	if _, _, err := s.CreateV1StartCommand(ctx, recoveryInput); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := s.ClaimV1StartDelivery(ctx, recoveryInput.CMSCommandID); err != nil || !claimed {
		t.Fatalf("recovery pending claim=%v err=%v", claimed, err)
	}
	if err := s.RecoverV1CommandDelivery(ctx); err != nil {
		t.Fatal(err)
	}
	if recovered, err := s.GetV1Command(ctx, recoveryInput.CMSCommandID); err != nil || recovered.State != "PERSISTED" {
		t.Fatalf("pending recovery=%#v err=%v", recovered, err)
	}
	if _, claimed, err := s.ClaimV1StartDelivery(ctx, recoveryInput.CMSCommandID); err != nil || !claimed {
		t.Fatalf("attempt claim=%v err=%v", claimed, err)
	}
	if _, err := s.BeginV1CommandDelivery(ctx, recoveryInput.CMSCommandID); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverV1CommandDelivery(ctx); err != nil {
		t.Fatal(err)
	}
	if recovered, err := s.GetV1Command(ctx, recoveryInput.CMSCommandID); err != nil || recovered.State != "AMBIGUOUS" {
		t.Fatalf("attempt recovery=%#v err=%v", recovered, err)
	}
	if loaded, err := s.GetV1TransactionByStartIntent(ctx, commandInput.CMSStartIntentID); err != nil || loaded.OCPPTransactionID != ocppTransactionID {
		t.Fatalf("transaction restart lookup=%#v err=%v", loaded, err)
	}

	if err = s.RecordV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 1, true, now); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 2, true, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 1, false, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordV1ConnectorStatus(ctx, V1ConnectorRuntime{ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: 1, Status: "Charging", ErrorCode: "NoError", ObservedAt: &now}); err != nil {
		t.Fatal(err)
	}
	runtime, err := s.GetV1ChargerRuntime(ctx, input.ChargerOCPPIdentity)
	if err != nil || runtime.ConnectionState != "ONLINE" || len(runtime.Connectors) != 1 || runtime.Connectors[0].Status != "Charging" {
		t.Fatalf("runtime=%#v err=%v", runtime, err)
	}
	if err = s.ResetV1ConnectionRuntime(ctx); err != nil {
		t.Fatal(err)
	}
	runtime, err = s.GetV1ChargerRuntime(ctx, input.ChargerOCPPIdentity)
	if err != nil || runtime.ConnectionState != "UNKNOWN" || runtime.Connectors[0].Status != "Charging" {
		t.Fatalf("restart runtime=%#v err=%v", runtime, err)
	}
}

func TestV1ConnectionRuntimeRestartAcceptsNewProcessGeneration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for disposable PostgreSQL restart regression")
	}
	ctx := context.Background()
	s, err := NewPostgresStore(config.Config{DatabaseURL: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.db.Close() })

	input := V1MappingInput{
		CPOID:               NewUUIDString(),
		CMSChargerID:        NewUUIDString(),
		ChargerOCPPIdentity: "CP-RESTART-" + NewUUIDString()[:8],
		Enabled:             true,
		CorrelationID:       "restart-generation-test",
		RequestDigest:       "restart-generation-digest",
	}
	if _, existed, err := s.SyncV1Mapping(ctx, input); err != nil || existed {
		t.Fatalf("sync mapping existed=%v err=%v", existed, err)
	}
	t.Cleanup(func() {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_fact_outbox WHERE aggregate_key=$1`, input.ChargerOCPPIdentity)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_charger_runtime WHERE charger_ocpp_identity=$1`, input.ChargerOCPPIdentity)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_mapping_audit WHERE cms_charger_id=$1::uuid`, input.CMSChargerID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_charger_mappings WHERE cms_charger_id=$1::uuid`, input.CMSChargerID)
	})

	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := s.RecordV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 3, true, now); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetV1ConnectionRuntime(ctx); err != nil {
		t.Fatal(err)
	}
	runtime, err := s.GetV1ChargerRuntime(ctx, input.ChargerOCPPIdentity)
	if err != nil || runtime.ConnectionState != "UNKNOWN" || runtime.ConnectionGeneration != 0 || runtime.ConnectionSequence != 2 {
		t.Fatalf("startup reset runtime=%#v err=%v", runtime, err)
	}
	if err := s.RecordV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 1, true, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 2, true, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 1, false, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	runtime, err = s.GetV1ChargerRuntime(ctx, input.ChargerOCPPIdentity)
	if err != nil || runtime.ConnectionState != "ONLINE" || runtime.ConnectionGeneration != 2 || runtime.ConnectionSequence != 4 {
		t.Fatalf("current runtime=%#v err=%v", runtime, err)
	}

	rows, err := s.db.QueryContext(ctx, `SELECT sequence,payload->>'connection_state',payload->>'connection_generation' FROM v1_fact_outbox WHERE aggregate_key=$1 AND fact_type='charger.connection.updated' ORDER BY sequence`, input.ChargerOCPPIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []struct {
		sequence   int64
		state      string
		generation string
	}
	for rows.Next() {
		var item struct {
			sequence   int64
			state      string
			generation string
		}
		if err := rows.Scan(&item.sequence, &item.state, &item.generation); err != nil {
			t.Fatal(err)
		}
		got = append(got, item)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		sequence   int64
		state      string
		generation string
	}{
		{sequence: 1, state: "ONLINE", generation: "3"},
		{sequence: 2, state: "UNKNOWN", generation: "0"},
		{sequence: 3, state: "ONLINE", generation: "1"},
		{sequence: 4, state: "ONLINE", generation: "2"},
	}
	if len(got) != len(want) {
		t.Fatalf("connection facts=%#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("connection fact %d=%#v want=%#v", i, got[i], want[i])
		}
	}
}

func TestV1CurrentConnectionRenewal(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for disposable PostgreSQL liveness regression")
	}
	ctx := context.Background()
	s, err := NewPostgresStore(config.Config{DatabaseURL: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.db.Close() })

	input := V1MappingInput{
		CPOID:               NewUUIDString(),
		CMSChargerID:        NewUUIDString(),
		ChargerOCPPIdentity: "CP-LIVENESS-" + NewUUIDString()[:8],
		Enabled:             true,
		CorrelationID:       "connection-liveness-test",
		RequestDigest:       "connection-liveness-digest",
	}
	if _, existed, err := s.SyncV1Mapping(ctx, input); err != nil || existed {
		t.Fatalf("sync mapping existed=%v err=%v", existed, err)
	}
	t.Cleanup(func() {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_fact_outbox WHERE aggregate_key=$1`, input.ChargerOCPPIdentity)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_charger_runtime WHERE charger_ocpp_identity=$1`, input.ChargerOCPPIdentity)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_mapping_audit WHERE cms_charger_id=$1::uuid`, input.CMSChargerID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_charger_mappings WHERE cms_charger_id=$1::uuid`, input.CMSChargerID)
	})

	first := time.Now().UTC().Truncate(time.Microsecond)
	if err := s.RecordV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 1, true, first); err != nil {
		t.Fatal(err)
	}
	second := first.Add(time.Minute)
	if err := s.RenewCurrentV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 1, second); err != nil {
		t.Fatal(err)
	}
	third := second.Add(time.Minute)
	if err := s.RenewCurrentV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 1, third); err != nil {
		t.Fatal(err)
	}
	runtime, err := s.GetV1ChargerRuntime(ctx, input.ChargerOCPPIdentity)
	if err != nil || runtime.ConnectionState != "ONLINE" || runtime.ConnectionGeneration != 1 || runtime.ConnectionSequence != 3 || runtime.LastObservedAt == nil || !runtime.LastObservedAt.Equal(third) {
		t.Fatalf("renewed runtime=%#v err=%v", runtime, err)
	}
	if err := s.RenewCurrentV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 1, second); err != nil {
		t.Fatal(err)
	}
	runtime, err = s.GetV1ChargerRuntime(ctx, input.ChargerOCPPIdentity)
	if err != nil || runtime.ConnectionSequence != 3 || runtime.LastObservedAt == nil || !runtime.LastObservedAt.Equal(third) {
		t.Fatalf("older renewal changed runtime=%#v err=%v", runtime, err)
	}

	if err := s.RecordV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 1, false, third.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.RenewCurrentV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 1, third.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	runtime, err = s.GetV1ChargerRuntime(ctx, input.ChargerOCPPIdentity)
	if err != nil || runtime.ConnectionState != "OFFLINE" || runtime.ConnectionGeneration != 1 || runtime.ConnectionSequence != 4 {
		t.Fatalf("offline renewal runtime=%#v err=%v", runtime, err)
	}

	if err := s.RecordV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 2, true, third.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.RenewCurrentV1ChargerConnection(ctx, input.ChargerOCPPIdentity, 1, third.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	runtime, err = s.GetV1ChargerRuntime(ctx, input.ChargerOCPPIdentity)
	if err != nil || runtime.ConnectionState != "ONLINE" || runtime.ConnectionGeneration != 2 || runtime.ConnectionSequence != 5 {
		t.Fatalf("stale-generation renewal runtime=%#v err=%v", runtime, err)
	}

	if err := s.RenewCurrentV1ChargerConnection(ctx, "CP-LIVENESS-UNKNOWN", 1, second); err != ErrV1MappingNotFound {
		t.Fatalf("unmapped renewal err=%v", err)
	}
	disabled := input
	disabled.CPOID, disabled.CMSChargerID = NewUUIDString(), NewUUIDString()
	disabled.ChargerOCPPIdentity = "CP-LIVENESS-DISABLED-" + NewUUIDString()[:8]
	disabled.Enabled, disabled.RequestDigest = false, "connection-liveness-disabled"
	if _, existed, err := s.SyncV1Mapping(ctx, disabled); err != nil || existed {
		t.Fatalf("sync disabled mapping existed=%v err=%v", existed, err)
	}
	t.Cleanup(func() {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_charger_runtime WHERE charger_ocpp_identity=$1`, disabled.ChargerOCPPIdentity)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_mapping_audit WHERE cms_charger_id=$1::uuid`, disabled.CMSChargerID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM v1_charger_mappings WHERE cms_charger_id=$1::uuid`, disabled.CMSChargerID)
	})
	if err := s.RenewCurrentV1ChargerConnection(ctx, disabled.ChargerOCPPIdentity, 1, second); err != ErrV1CredentialRejected {
		t.Fatalf("disabled renewal err=%v", err)
	}

	var facts int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM v1_fact_outbox WHERE aggregate_key=$1 AND fact_type='charger.connection.updated'`, input.ChargerOCPPIdentity).Scan(&facts); err != nil {
		t.Fatal(err)
	}
	if facts != 5 {
		t.Fatalf("connection facts=%d, want 5", facts)
	}
}
