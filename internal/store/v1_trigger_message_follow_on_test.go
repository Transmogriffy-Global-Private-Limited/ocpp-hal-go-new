package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestV1TriggerMessageFollowOnActionsAndConnectorScope(t *testing.T) {
	for _, test := range []struct {
		action          string
		connectorScoped bool
	}{
		{"BootNotification", false}, {"DiagnosticsStatusNotification", false}, {"FirmwareStatusNotification", false},
		{"Heartbeat", false}, {"MeterValues", true}, {"StatusNotification", true}, {"Authorize", false},
	} {
		t.Run(test.action, func(t *testing.T) {
			if V1TriggerMessageAction(test.action) != (test.action != "Authorize") || V1TriggerMessageConnectorScoped(test.action) != test.connectorScoped {
				t.Fatalf("action mapping %q = allowed=%t connector_scoped=%t", test.action, V1TriggerMessageAction(test.action), V1TriggerMessageConnectorScoped(test.action))
			}
		})
	}
}

func acceptedTriggerMessageTraceInput(at time.Time) V1TraceEventInput {
	return V1TraceEventInput{Source: "HAL", Target: "CMS", Category: "CHARGER_OPERATION_OCPP", Protocol: "OCPP1.6", Phase: "STARTING", Summary: "CPO operation OCPP CALLRESULT", OccurredAt: at, Data: map[string]any{"unique_id": "call-1", "action": "TriggerMessage", "message_type": "CALLRESULT", "payload": map[string]any{"status": "Accepted"}}}
}

func appendAcceptedTriggerMessage(t *testing.T, traces *V1MemoryStore, traceID, action string, at time.Time) {
	t.Helper()
	if err := traces.AppendV1AcceptedTriggerMessageTrace(context.Background(), traceID, acceptedTriggerMessageTraceInput(at), action); err != nil {
		t.Fatal(err)
	}
}

func TestV1AcceptedTriggerMessageTraceAndWindowAreAtomic(t *testing.T) {
	acceptedAt := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	traces := NewV1MemoryStore()
	if _, err := traces.EnsureV1Trace(context.Background(), V1Trace{TraceID: "trace-accepted", ChargerOCPPIdentity: "CP-1"}); err != nil {
		t.Fatal(err)
	}
	appendAcceptedTriggerMessage(t, traces, "trace-accepted", "Heartbeat", acceptedAt)
	events, err := traces.ListV1TraceEvents(context.Background(), "trace-accepted", time.Time{}, "", 10)
	if err != nil || len(events) != 1 || events[0].Category != "CHARGER_OPERATION_OCPP" {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	traces.mu.Lock()
	window := traces.followOnWindows["trace-accepted"]
	traces.mu.Unlock()
	if window == nil || window.State != "OPEN" || !window.AcceptedAt.Equal(acceptedAt) {
		t.Fatalf("window=%#v", window)
	}
	appendAcceptedTriggerMessage(t, traces, "trace-accepted", "Heartbeat", acceptedAt)
	events, err = traces.ListV1TraceEvents(context.Background(), "trace-accepted", time.Time{}, "", 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("duplicate Accepted event was not idempotent: events=%#v err=%v", events, err)
	}
}

func TestV1AcceptedTriggerMessageWindowFailureRollsBackTrace(t *testing.T) {
	traces := NewV1MemoryStore()
	if _, err := traces.EnsureV1Trace(context.Background(), V1Trace{TraceID: "trace-failed", ChargerOCPPIdentity: "CP-1"}); err != nil {
		t.Fatal(err)
	}
	traces.followOnWindowErr = errors.New("injected window failure")
	err := traces.AppendV1AcceptedTriggerMessageTrace(context.Background(), "trace-failed", acceptedTriggerMessageTraceInput(time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)), "Heartbeat")
	if err == nil {
		t.Fatal("expected atomic window failure")
	}
	events, listErr := traces.ListV1TraceEvents(context.Background(), "trace-failed", time.Time{}, "", 10)
	if listErr != nil || len(events) != 0 {
		t.Fatalf("committed accepted evidence after window failure: events=%#v err=%v", events, listErr)
	}
	if traces.followOnWindows["trace-failed"] != nil {
		t.Fatal("committed follow-on window after injected failure")
	}
}

func TestV1TriggerMessageFollowOnWindowMatchesAcceptedEvidenceWithoutOperationCompletion(t *testing.T) {
	acceptedAt := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	traces := NewV1MemoryStore()
	for _, trace := range []V1Trace{{TraceID: "trace-heartbeat", ChargerOCPPIdentity: "CP-1"}, {TraceID: "trace-meter", ChargerOCPPIdentity: "CP-1", OCPPConnectorNumber: 2}} {
		if _, err := traces.EnsureV1Trace(context.Background(), trace); err != nil {
			t.Fatal(err)
		}
	}
	appendAcceptedTriggerMessage(t, traces, "trace-heartbeat", "Heartbeat", acceptedAt)
	appendAcceptedTriggerMessage(t, traces, "trace-meter", "MeterValues", acceptedAt)

	for _, test := range []struct {
		name             string
		action, identity string
		connector        int
		observedAt       time.Time
		want             int
	}{
		{"heartbeat", "Heartbeat", "CP-1", 0, acceptedAt.Add(time.Second), 1},
		{"wrong connector", "MeterValues", "CP-1", 1, acceptedAt.Add(time.Second), 0},
		{"matching connector", "MeterValues", "CP-1", 2, acceptedAt.Add(time.Second), 1},
		{"wrong charger", "StatusNotification", "CP-2", 2, acceptedAt.Add(time.Second), 0},
		{"at acceptance", "Heartbeat", "CP-1", 0, acceptedAt, 0},
		{"post deadline", "Heartbeat", "CP-1", 0, acceptedAt.Add(time.Minute + time.Nanosecond), 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := traces.RecordV1TriggerMessageFollowOn(context.Background(), test.identity, test.action, test.connector, test.observedAt)
			if err != nil || got != test.want {
				t.Fatalf("matches=%d want=%d err=%v", got, test.want, err)
			}
		})
	}
}

func TestV1TriggerMessageFollowOnWindowsPreserveOverlapsButClosureIsFinal(t *testing.T) {
	acceptedAt := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	traces := NewV1MemoryStore()
	for _, traceID := range []string{"trace-one", "trace-two"} {
		if _, err := traces.EnsureV1Trace(context.Background(), V1Trace{TraceID: traceID, ChargerOCPPIdentity: "CP-1"}); err != nil {
			t.Fatal(err)
		}
		appendAcceptedTriggerMessage(t, traces, traceID, "Heartbeat", acceptedAt)
	}
	if matched, err := traces.RecordV1TriggerMessageFollowOn(context.Background(), "CP-1", "Heartbeat", 0, acceptedAt.Add(time.Second)); err != nil || matched != 2 {
		t.Fatalf("overlapping positive matches=%d err=%v", matched, err)
	}
	if closed, err := traces.CloseV1TriggerMessageFollowOnWindows(context.Background(), acceptedAt.Add(time.Minute), 32); err != nil || closed != 0 {
		t.Fatalf("closed observed windows=%d err=%v", closed, err)
	}

	if _, err := traces.EnsureV1Trace(context.Background(), V1Trace{TraceID: "trace-closed", ChargerOCPPIdentity: "CP-1"}); err != nil {
		t.Fatal(err)
	}
	appendAcceptedTriggerMessage(t, traces, "trace-closed", "Heartbeat", acceptedAt)
	if closed, err := traces.CloseV1TriggerMessageFollowOnWindows(context.Background(), acceptedAt.Add(time.Minute), 32); err != nil || closed != 1 {
		t.Fatalf("closed=%d err=%v", closed, err)
	}
	if matched, err := traces.RecordV1TriggerMessageFollowOn(context.Background(), "CP-1", "Heartbeat", 0, acceptedAt.Add(time.Second)); err != nil || matched != 0 {
		t.Fatalf("closure was invalidated: matches=%d err=%v", matched, err)
	}
	events, err := traces.ListV1TraceEvents(context.Background(), "trace-closed", time.Time{}, "", 10)
	if err != nil || len(events) != 2 || events[0].Category != "CHARGER_OPERATION_FOLLOW_ON_CLOSED" {
		t.Fatalf("events=%#v err=%v", events, err)
	}
}
