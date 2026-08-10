package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/OCPPHAL_Go/internal/config"
)

func TestV1PostgresStoreDurabilityAndRuntime(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL v1 integration test")
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
	tx, duplicate, err := s.MaterializeV1Start(ctx, V1StartMaterialization{ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: 1, IDTag: commandInput.IDTag, MeterStartWh: 12345, ActualStartedAt: now, OCPPTransactionID: ocppTransactionID})
	if err != nil || duplicate {
		t.Fatalf("materialize=%#v duplicate=%v err=%v", tx, duplicate, err)
	}
	retransmit, duplicate, err := s.MaterializeV1Start(ctx, V1StartMaterialization{ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: 1, IDTag: commandInput.IDTag, MeterStartWh: 12345, ActualStartedAt: now, OCPPTransactionID: RandomTransactionID()})
	if err != nil || !duplicate || retransmit.OCPPTransactionID != ocppTransactionID {
		t.Fatalf("retransmit=%#v duplicate=%v err=%v", retransmit, duplicate, err)
	}
	if tx.StopDeadlineAt == nil || !tx.StopDeadlineAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("deadline=%v", tx.StopDeadlineAt)
	}
	command, err = s.GetV1Command(ctx, commandInput.CMSCommandID)
	if err != nil || command.State != "MATERIALIZED" {
		t.Fatalf("materialized command=%#v err=%v", command, err)
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
