package store

import (
	"context"
	"testing"
	"time"
)

func TestV1TriggerMessageFollowOnActionsAndConnectorScope(t *testing.T) {
	for _, test := range []struct {
		action          string
		connectorScoped bool
	}{
		{"BootNotification", false},
		{"DiagnosticsStatusNotification", false},
		{"FirmwareStatusNotification", false},
		{"Heartbeat", false},
		{"MeterValues", true},
		{"StatusNotification", true},
		{"Authorize", false},
	} {
		t.Run(test.action, func(t *testing.T) {
			if V1TriggerMessageAction(test.action) != (test.action != "Authorize") || V1TriggerMessageConnectorScoped(test.action) != test.connectorScoped {
				t.Fatalf("action mapping %q = allowed=%t connector_scoped=%t", test.action, V1TriggerMessageAction(test.action), V1TriggerMessageConnectorScoped(test.action))
			}
		})
	}
}

func TestV1TriggerMessageFollowOnWindowMatchesAcceptedEvidenceWithoutOperationCompletion(t *testing.T) {
	acceptedAt := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	traces := NewV1MemoryStore()
	for _, trace := range []V1Trace{
		{TraceID: "trace-heartbeat", ChargerOCPPIdentity: "CP-1"},
		{TraceID: "trace-meter", ChargerOCPPIdentity: "CP-1", OCPPConnectorNumber: 2},
	} {
		if _, err := traces.EnsureV1Trace(context.Background(), trace); err != nil {
			t.Fatal(err)
		}
	}
	if err := traces.OpenV1TriggerMessageFollowOnWindow(context.Background(), "trace-heartbeat", "Heartbeat", acceptedAt); err != nil {
		t.Fatal(err)
	}
	if err := traces.OpenV1TriggerMessageFollowOnWindow(context.Background(), "trace-meter", "MeterValues", acceptedAt); err != nil {
		t.Fatal(err)
	}

	if matches, err := traces.RecordV1TriggerMessageFollowOn(context.Background(), "CP-1", "Heartbeat", 0, acceptedAt.Add(time.Second)); err != nil || matches != 1 {
		t.Fatalf("heartbeat matches=%d err=%v", matches, err)
	}
	if matches, err := traces.RecordV1TriggerMessageFollowOn(context.Background(), "CP-1", "MeterValues", 1, acceptedAt.Add(time.Second)); err != nil || matches != 0 {
		t.Fatalf("wrong connector matches=%d err=%v", matches, err)
	}
	if matches, err := traces.RecordV1TriggerMessageFollowOn(context.Background(), "CP-1", "MeterValues", 2, acceptedAt.Add(time.Second)); err != nil || matches != 1 {
		t.Fatalf("matching connector matches=%d err=%v", matches, err)
	}
	if matches, err := traces.RecordV1TriggerMessageFollowOn(context.Background(), "CP-1", "Heartbeat", 0, acceptedAt); err != nil || matches != 0 {
		t.Fatalf("pre-acceptance matches=%d err=%v", matches, err)
	}
	if matches, err := traces.RecordV1TriggerMessageFollowOn(context.Background(), "CP-1", "Heartbeat", 0, acceptedAt.Add(time.Minute+time.Nanosecond)); err != nil || matches != 0 {
		t.Fatalf("post-deadline matches=%d err=%v", matches, err)
	}
}

func TestV1TriggerMessageFollowOnWindowsPreserveOverlapsAndClosureCannotOutweighPositive(t *testing.T) {
	acceptedAt := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	traces := NewV1MemoryStore()
	for _, traceID := range []string{"trace-one", "trace-two"} {
		if _, err := traces.EnsureV1Trace(context.Background(), V1Trace{TraceID: traceID, ChargerOCPPIdentity: "CP-1"}); err != nil {
			t.Fatal(err)
		}
		if err := traces.OpenV1TriggerMessageFollowOnWindow(context.Background(), traceID, "Heartbeat", acceptedAt); err != nil {
			t.Fatal(err)
		}
	}
	if closed, err := traces.CloseV1TriggerMessageFollowOnWindows(context.Background(), acceptedAt.Add(time.Minute), 32); err != nil || closed != 2 {
		t.Fatalf("closed=%d err=%v", closed, err)
	}
	if matched, err := traces.RecordV1TriggerMessageFollowOn(context.Background(), "CP-1", "Heartbeat", 0, acceptedAt.Add(30*time.Second)); err != nil || matched != 2 {
		t.Fatalf("late-arriving positive matches=%d err=%v", matched, err)
	}
	for _, traceID := range []string{"trace-one", "trace-two"} {
		events, err := traces.ListV1TraceEvents(context.Background(), traceID, time.Time{}, "", 10)
		if err != nil || len(events) != 2 {
			t.Fatalf("trace=%s events=%#v err=%v", traceID, events, err)
		}
		seen := map[string]bool{}
		for _, event := range events {
			seen[event.Category] = true
		}
		if !seen["CHARGER_OPERATION_FOLLOW_ON"] || !seen["CHARGER_OPERATION_FOLLOW_ON_CLOSED"] {
			t.Fatalf("trace=%s categories=%#v", traceID, seen)
		}
	}
}
