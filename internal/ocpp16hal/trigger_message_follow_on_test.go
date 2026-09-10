package ocpp16hal

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

type followOnTraceStore struct {
	store.V1Store
	store.V1TraceStore
	recorded  int
	recordErr error
}

func (s *followOnTraceStore) RecordV1TriggerMessageFollowOn(_ context.Context, _, _ string, _ int, _ time.Time) (int, error) {
	s.recorded++
	return 1, s.recordErr
}

func (*followOnTraceStore) CloseV1TriggerMessageFollowOnWindows(context.Context, time.Time, int) (int, error) {
	return 0, nil
}

func TestRecordTriggerMessageFollowOnUsesIndexedWindowStore(t *testing.T) {
	traces := &followOnTraceStore{}
	hal := New(state.NewRegistry(), traces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	hal.recordTriggerMessageFollowOn("CP-1", "Heartbeat", 0, time.Date(2026, time.September, 10, 10, 0, 1, 0, time.UTC))
	if traces.recorded != 1 {
		t.Fatalf("recorded=%d", traces.recorded)
	}
}

func TestAcceptedTriggerMessageOpensWindowBeforeOperationCompletion(t *testing.T) {
	traces := store.NewV1MemoryStore()
	if _, err := traces.EnsureV1Trace(context.Background(), store.V1Trace{TraceID: "trace-1", ChargerOCPPIdentity: "CP-1"}); err != nil {
		t.Fatal(err)
	}
	observer := newOperationObserver(traces, nil)
	observer.byUnique["call-1"] = &observedOperation{traceID: "trace-1", chargerID: "CP-1", action: "TriggerMessage", uniqueID: "call-1", requestedMessage: "Heartbeat"}
	observer.received("call-1", "CALLRESULT", map[string]any{"status": "Accepted"})

	// No v1_charger_operations entry exists: the OCPP CALLRESULT itself is the
	// durable acceptance anchor, before later HTTP operation finalization.
	if matched, err := traces.RecordV1TriggerMessageFollowOn(context.Background(), "CP-1", "Heartbeat", 0, time.Now().UTC().Add(time.Millisecond)); err != nil || matched != 1 {
		t.Fatalf("matched=%d err=%v", matched, err)
	}
}

func TestTriggerMessageFollowOnTraceFailureDoesNotChangeHeartbeatAcknowledgement(t *testing.T) {
	traces := &followOnTraceStore{recordErr: context.DeadlineExceeded}
	hal := New(state.NewRegistry(), traces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	confirmation, err := hal.OnHeartbeat("CP-1", nil)
	if err != nil || confirmation == nil || traces.recorded != 1 {
		t.Fatalf("confirmation=%#v err=%v recorded=%d", confirmation, err, traces.recorded)
	}
}
