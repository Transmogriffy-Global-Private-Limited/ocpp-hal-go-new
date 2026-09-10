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
	expectations []store.V1TriggerMessageFollowOnExpectation
	events       []store.V1TraceEventInput
	appendErr    error
}

func (s *followOnTraceStore) ListV1TriggerMessageFollowOnExpectations(_ context.Context, _, _ string, _ int, _ bool, _, _ time.Time) ([]store.V1TriggerMessageFollowOnExpectation, error) {
	return s.expectations, nil
}
func (s *followOnTraceStore) AppendV1TraceEvent(_ context.Context, _ string, input store.V1TraceEventInput) error {
	s.events = append(s.events, input)
	return s.appendErr
}

func TestRecordTriggerMessageFollowOnWritesOnlySafeDiagnosticEvidence(t *testing.T) {
	traces := &followOnTraceStore{expectations: []store.V1TriggerMessageFollowOnExpectation{{TraceID: "trace-1", RequestedMessage: "Heartbeat"}}}
	hal := New(state.NewRegistry(), traces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	observedAt := time.Date(2026, time.September, 10, 10, 0, 1, 0, time.UTC)
	hal.recordTriggerMessageFollowOn("CP-1", "Heartbeat", 0, observedAt)
	if len(traces.events) != 1 {
		t.Fatalf("events=%#v", traces.events)
	}
	event := traces.events[0]
	if event.Category != "CHARGER_OPERATION_FOLLOW_ON" || !event.OccurredAt.Equal(observedAt) || event.Data.(map[string]any)["expected_message"] != "Heartbeat" || event.Data.(map[string]any)["observed_action"] != "Heartbeat" {
		t.Fatalf("event=%#v", event)
	}
}

func TestTriggerMessageFollowOnTraceFailureDoesNotChangeHeartbeatAcknowledgement(t *testing.T) {
	traces := &followOnTraceStore{expectations: []store.V1TriggerMessageFollowOnExpectation{{TraceID: "trace-1", RequestedMessage: "Heartbeat"}}, appendErr: context.DeadlineExceeded}
	hal := New(state.NewRegistry(), traces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	confirmation, err := hal.OnHeartbeat("CP-1", nil)
	if err != nil || confirmation == nil || len(traces.events) != 1 {
		t.Fatalf("confirmation=%#v err=%v events=%#v", confirmation, err, traces.events)
	}
}
