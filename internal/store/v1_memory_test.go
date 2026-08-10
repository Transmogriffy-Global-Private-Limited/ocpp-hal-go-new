package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestV1MemoryStoreMaterializesOneCredentialBoundTransaction(t *testing.T) {
	ctx := context.Background()
	s := NewV1MemoryStore()
	now := time.Now().UTC()
	energyLimit := int64(4800)
	maxDuration := int64(3600)

	command, existed, err := s.CreateV1StartCommand(ctx, V1StartCommandInput{
		CMSCommandID:        "8bbcc0b3-5b66-4f3c-a6c2-1dc36400f201",
		RequestDigest:       "start-body-digest",
		CPOID:               "3340d9c1-9579-4d11-832f-169ab4700179",
		CMSStartIntentID:    "4f372a28-e02f-4f1a-8a0a-74d792a18d63",
		CMSChargerID:        "b2c8e142-4034-43d2-9e2e-a93a4c93aa09",
		CMSConnectorID:      "ed2d4036-0b2e-4b1c-84b3-351364a98680",
		ChargerOCPPIdentity: "CP-V1-001",
		OCPPConnectorNumber: 1,
		IDTag:               "appv1_test_001",
		CredentialExpiresAt: now.Add(5 * time.Minute),
		CommandExpiresAt:    now.Add(6 * time.Minute),
		EnergyLimitWh:       &energyLimit,
		MaxDurationSeconds:  &maxDuration,
	})
	if err != nil || existed {
		t.Fatalf("create start command = %#v, existed=%v, err=%v", command, existed, err)
	}

	tx, duplicate, err := s.MaterializeV1Start(ctx, V1StartMaterialization{
		ChargerOCPPIdentity: "CP-V1-001",
		OCPPConnectorNumber: 1,
		IDTag:               "appv1_test_001",
		MeterStartWh:        1000,
		ActualStartedAt:     now,
		OCPPTransactionID:   44,
	})
	if err != nil || duplicate {
		t.Fatalf("materialize start = %#v, duplicate=%v, err=%v", tx, duplicate, err)
	}
	if tx.EnergyLimitWh == nil || *tx.EnergyLimitWh != energyLimit {
		t.Fatalf("energy limit = %v, want %d", tx.EnergyLimitWh, energyLimit)
	}
	if tx.StopDeadlineAt == nil || !tx.StopDeadlineAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("stop deadline = %v, want %v", tx.StopDeadlineAt, now.Add(time.Hour))
	}

	replayed, duplicate, err := s.MaterializeV1Start(ctx, V1StartMaterialization{
		ChargerOCPPIdentity: "CP-V1-001",
		OCPPConnectorNumber: 1,
		IDTag:               "appv1_test_001",
		MeterStartWh:        1000,
		ActualStartedAt:     now,
		OCPPTransactionID:   44,
	})
	if err != nil || !duplicate || replayed.HALTransactionID != tx.HALTransactionID {
		t.Fatalf("duplicate start = %#v, duplicate=%v, err=%v", replayed, duplicate, err)
	}

	_, _, err = s.MaterializeV1Start(ctx, V1StartMaterialization{
		ChargerOCPPIdentity: "CP-V1-OTHER",
		OCPPConnectorNumber: 1,
		IDTag:               "appv1_test_001",
		MeterStartWh:        1000,
		ActualStartedAt:     now,
		OCPPTransactionID:   45,
	})
	if !errors.Is(err, ErrV1CredentialRejected) {
		t.Fatalf("wrong charger start error = %v, want credential rejection", err)
	}
}

func TestV1MemoryStoreCommandsAreIdempotentAndStopIsUnified(t *testing.T) {
	ctx := context.Background()
	s := NewV1MemoryStore()
	now := time.Now().UTC()
	limit := int64(10)
	_, _, err := s.CreateV1StartCommand(ctx, V1StartCommandInput{CMSCommandID: "96bd543e-ba5e-4df8-b8ee-bad42ce5b784", RequestDigest: "digest", CPOID: "48c900cd-c22a-4f5d-a0b3-56d63c6d2833", CMSStartIntentID: "69964322-66e8-4fcd-b8e1-10002c74fdd2", CMSChargerID: "477f3730-16c3-4c06-a3e2-c1262fd8f71d", CMSConnectorID: "e1bcf85f-c7ff-4d06-8836-2e1b92cae853", ChargerOCPPIdentity: "CP-V1-STOP", OCPPConnectorNumber: 1, IDTag: "appv1_stop", CredentialExpiresAt: now.Add(time.Minute), CommandExpiresAt: now.Add(time.Minute), EnergyLimitWh: &limit})
	if err != nil {
		t.Fatal(err)
	}
	tx, _, err := s.MaterializeV1Start(ctx, V1StartMaterialization{ChargerOCPPIdentity: "CP-V1-STOP", OCPPConnectorNumber: 1, IDTag: "appv1_stop", MeterStartWh: 100, ActualStartedAt: now, OCPPTransactionID: 99})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.UpdateV1Meter(ctx, tx.HALTransactionID, 99, 110, now.Add(time.Second))
	if err != nil || updated.LatestMeterWh == nil || *updated.LatestMeterWh != 110 || updated.MeterSequence != 1 {
		t.Fatalf("update meter = %#v, err=%v", updated, err)
	}
	stopped, created, err := s.RequestV1Stop(ctx, tx.HALTransactionID, "ENERGY_LIMIT", "energy_limit")
	if err != nil || !created || stopped.StopState != "PENDING_DELIVERY" {
		t.Fatalf("request stop = %#v, created=%v, err=%v", stopped, created, err)
	}
	stopped, created, err = s.RequestV1Stop(ctx, tx.HALTransactionID, "CUSTOMER", "user_requested")
	if err != nil || created || stopped.RequestedStopInitiator != "ENERGY_LIMIT" {
		t.Fatalf("duplicate stop = %#v, created=%v, err=%v", stopped, created, err)
	}
	completed, err := s.CompleteV1Transaction(ctx, tx.HALTransactionID, 125, "Remote", now.Add(2*time.Second))
	if err != nil || completed.OCPPStopReason != "Remote" || completed.MeterStopWh == nil || *completed.MeterStopWh != 125 {
		t.Fatalf("complete = %#v, err=%v", completed, err)
	}
}
