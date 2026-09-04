package store

import (
	"context"
	"errors"
	"testing"
)

func TestV1ChargerOperationIdempotencyAndSingleDeliveryClaim(t *testing.T) {
	store := NewV1MemoryStore()
	input := V1ChargerOperationInput{
		CMSOperationID:      "77bb773c-67ed-4afc-a952-40604176723d",
		RequestDigest:       "digest-a",
		CPOID:               "f1c8a1f1-9f48-42f7-b328-7f1234567890",
		CMSChargerID:        "c1c8a1f1-9f48-42f7-b328-7f1234567890",
		ChargerOCPPIdentity: "CP-001",
		Kind:                "CLEAR_CACHE",
		CorrelationID:       "bcff6f49-9b73-4a68-85e9-b8545b2a47cf",
	}

	created, replay, err := store.CreateV1ChargerOperation(context.Background(), input)
	if err != nil || replay || created.State != "PERSISTED" {
		t.Fatalf("first create = %#v, replay=%t, err=%v", created, replay, err)
	}
	retried, replay, err := store.CreateV1ChargerOperation(context.Background(), input)
	if err != nil || !replay || retried.HALOperationID != created.HALOperationID {
		t.Fatalf("idempotent create = %#v, replay=%t, err=%v", retried, replay, err)
	}

	claimed, deliver, err := store.ClaimV1ChargerOperationDelivery(context.Background(), input.CMSOperationID)
	if err != nil || !deliver || claimed.DeliveryAttempts != 1 || claimed.State != "DELIVERY_ATTEMPTED" {
		t.Fatalf("first claim = %#v, deliver=%t, err=%v", claimed, deliver, err)
	}
	claimed, deliver, err = store.ClaimV1ChargerOperationDelivery(context.Background(), input.CMSOperationID)
	if err != nil || deliver || claimed.DeliveryAttempts != 1 {
		t.Fatalf("second claim must not replay physical delivery: %#v, deliver=%t, err=%v", claimed, deliver, err)
	}
	completed, err := store.MarkV1ChargerOperationDelivery(context.Background(), input.CMSOperationID, "RECONCILIATION_REQUIRED", "", "delivery_ambiguous")
	if err != nil || completed.CompletedAt == nil || completed.ErrorCategory != "delivery_ambiguous" {
		t.Fatalf("mark ambiguous delivery = %#v, err=%v", completed, err)
	}
}

func TestV1ChargerOperationRejectsDifferentIdempotencyPayload(t *testing.T) {
	store := NewV1MemoryStore()
	input := V1ChargerOperationInput{CMSOperationID: "67dc30d1-3347-4622-bafc-bd4ee53d84ef", RequestDigest: "first", Kind: "CLEAR_CACHE"}
	if _, _, err := store.CreateV1ChargerOperation(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	input.RequestDigest = "second"
	if _, _, err := store.CreateV1ChargerOperation(context.Background(), input); !errors.Is(err, ErrV1IdempotencyConflict) {
		t.Fatalf("err = %v, want idempotency conflict", err)
	}
}
