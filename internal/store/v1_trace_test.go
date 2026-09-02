package store

import (
	"context"
	"testing"
	"time"
)

func TestV1TraceEventsCursorIsScopedStableAndBounded(t *testing.T) {
	s := NewV1MemoryStore()
	ctx := context.Background()
	at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for _, id := range []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"} {
		if _, err := s.EnsureV1Trace(ctx, V1Trace{TraceID: id, CPOID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", ChargerOCPPIdentity: "cp", OCPPConnectorNumber: 1}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := s.AppendV1TraceEvent(ctx, "11111111-1111-4111-8111-111111111111", V1TraceEventInput{Source: "HAL", Target: "CMS", Category: "TEST", Protocol: "TEST", Phase: "CHARGING", Summary: "event", OccurredAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AppendV1TraceEvent(ctx, "22222222-2222-4222-8222-222222222222", V1TraceEventInput{Source: "HAL", Target: "CMS", Category: "TEST", Protocol: "TEST", Phase: "CHARGING", Summary: "foreign", OccurredAt: at}); err != nil {
		t.Fatal(err)
	}
	first, err := s.ListV1TraceEvents(ctx, "11111111-1111-4111-8111-111111111111", time.Time{}, "", 2)
	if err != nil || len(first) != 2 {
		t.Fatalf("first=%d err=%v", len(first), err)
	}
	second, err := s.ListV1TraceEvents(ctx, "11111111-1111-4111-8111-111111111111", first[1].OccurredAt, first[1].EventID, 2)
	if err != nil || len(second) != 1 {
		t.Fatalf("second=%d err=%v", len(second), err)
	}
	if first[0].EventID == second[0].EventID || first[1].EventID == second[0].EventID {
		t.Fatal("cursor overlapped")
	}
	if _, err := s.ListV1TraceEvents(ctx, "11111111-1111-4111-8111-111111111111", at, "", 2); err != ErrV1InvalidEvidence {
		t.Fatalf("invalid cursor err=%v", err)
	}
}

func TestV1TraceForTransactionBindsTheExistingCMSStartRoot(t *testing.T) {
	s := NewV1MemoryStore()
	ctx := context.Background()
	const traceID = "33333333-3333-4333-8333-333333333333"
	if _, err := s.EnsureV1Trace(ctx, V1Trace{
		TraceID:             traceID,
		CPOID:               "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		CMSStartIntentID:    "44444444-4444-4444-8444-444444444444",
		CMSCommandID:        "55555555-5555-4555-8555-555555555555",
		ChargerOCPPIdentity: "cp-a",
		OCPPConnectorNumber: 1,
	}); err != nil {
		t.Fatal(err)
	}
	trace, err := s.EnsureV1TraceForTransaction(ctx, &V1Transaction{
		HALTransactionID:    "66666666-6666-4666-8666-666666666666",
		CMSStartIntentID:    "44444444-4444-4444-8444-444444444444",
		CMSCommandID:        "55555555-5555-4555-8555-555555555555",
		CPOID:               "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		ChargerOCPPIdentity: "cp-a",
		OCPPConnectorNumber: 1,
		OCPPTransactionID:   7654321,
	})
	if err != nil {
		t.Fatal(err)
	}
	if trace.TraceID != traceID || trace.HALTransactionID == "" || trace.OCPPTransactionID == nil || *trace.OCPPTransactionID != 7654321 {
		t.Fatalf("trace was not bound to the CMS root: %#v", trace)
	}
}

func TestV1TraceRetentionDeletesOnlyBoundedExpiredEvidence(t *testing.T) {
	s := NewV1MemoryStore()
	ctx := context.Background()
	old := time.Now().UTC().Add(-48 * time.Hour)
	for _, id := range []string{"77777777-7777-4777-8777-777777777777", "88888888-8888-4888-8888-888888888888"} {
		if _, err := s.EnsureV1Trace(ctx, V1Trace{TraceID: id, CPOID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", ChargerOCPPIdentity: "cp", OCPPConnectorNumber: 1, CreatedAt: old}); err != nil {
			t.Fatal(err)
		}
	}
	deleted, err := s.DeleteV1TracesBefore(ctx, time.Now().UTC().Add(-24*time.Hour), 1)
	if err != nil || deleted != 1 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	_, firstErr := s.GetV1Trace(ctx, "77777777-7777-4777-8777-777777777777")
	_, secondErr := s.GetV1Trace(ctx, "88888888-8888-4888-8888-888888888888")
	if (firstErr == nil) == (secondErr == nil) {
		t.Fatal("one expired trace should have been removed while the bounded remainder remains")
	}
}

func TestV1TraceConnectorAssociationPrefersActiveAndExpiresOldPostStop(t *testing.T) {
	s := NewV1MemoryStore()
	ctx := context.Background()
	oldID := "99999999-9999-4999-8999-999999999999"
	activeID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	postStopID := "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	for _, trace := range []V1Trace{
		{TraceID: oldID, CPOID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", HALTransactionID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", ChargerOCPPIdentity: "cp-a", OCPPConnectorNumber: 1},
		{TraceID: activeID, CPOID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", HALTransactionID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", ChargerOCPPIdentity: "cp-a", OCPPConnectorNumber: 1},
		{TraceID: postStopID, CPOID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", HALTransactionID: "ffffffff-ffff-4fff-8fff-ffffffffffff", ChargerOCPPIdentity: "cp-a", OCPPConnectorNumber: 1},
	} {
		if _, err := s.EnsureV1Trace(ctx, trace); err != nil {
			t.Fatal(err)
		}
	}
	expired := time.Now().UTC().Add(-v1TracePostStopAssociationWindow - time.Second)
	recent := time.Now().UTC()
	s.transactions["cccccccc-cccc-4ccc-8ccc-cccccccccccc"] = &V1Transaction{HALTransactionID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", CompletedAt: &expired}
	s.transactions["dddddddd-dddd-4ddd-8ddd-dddddddddddd"] = &V1Transaction{HALTransactionID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd"}
	s.transactions["ffffffff-ffff-4fff-8fff-ffffffffffff"] = &V1Transaction{HALTransactionID: "ffffffff-ffff-4fff-8fff-ffffffffffff", CompletedAt: &recent}
	trace, err := s.FindV1TraceForConnector(ctx, "cp-a", 1)
	if err != nil || trace.TraceID != activeID {
		t.Fatalf("active connector association = %#v, %v", trace, err)
	}
	delete(s.transactions, "dddddddd-dddd-4ddd-8ddd-dddddddddddd")
	trace, err = s.FindV1TraceForConnector(ctx, "cp-a", 1)
	if err != nil || trace.TraceID != postStopID {
		t.Fatalf("recent post-stop connector association = %#v, %v", trace, err)
	}
	s.transactions["ffffffff-ffff-4fff-8fff-ffffffffffff"].CompletedAt = &expired
	if _, err := s.FindV1TraceForConnector(ctx, "cp-a", 1); err != ErrV1TransactionNotFound {
		t.Fatalf("expired post-stop association err=%v", err)
	}
}

func TestV1TracePersistenceSanitizesUnsupportedFields(t *testing.T) {
	s := NewV1MemoryStore()
	ctx := context.Background()
	const traceID = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	if _, err := s.EnsureV1Trace(ctx, V1Trace{TraceID: traceID, CPOID: "ffffffff-ffff-4fff-8fff-ffffffffffff", ChargerOCPPIdentity: "cp", OCPPConnectorNumber: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendV1TraceEvent(ctx, traceID, V1TraceEventInput{Source: "HAL", Target: "CMS", Category: "TEST", Protocol: "HTTP", Phase: "CHARGING", Summary: "sanitized", Data: map[string]any{"meter_wh": int64(42), "id_tag": "must-not-persist", "authorization": "must-not-persist"}}); err != nil {
		t.Fatal(err)
	}
	events, err := s.ListV1TraceEvents(ctx, traceID, time.Time{}, "", 1)
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
	data, ok := events[0].Data.(map[string]any)
	if !ok || data["meter_wh"] != int64(42) || data["id_tag"] != nil || data["authorization"] != nil {
		t.Fatalf("unsafe persisted trace data: %#v", events[0].Data)
	}
}
