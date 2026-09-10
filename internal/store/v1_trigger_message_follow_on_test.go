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

func TestV1TriggerMessageFollowOnExpectationMatching(t *testing.T) {
	acceptedAt := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	store := NewV1MemoryStore()
	addExpectation := func(id, traceID, identity, action string, connector int, accepted bool) {
		store.operations[id] = &V1ChargerOperation{CMSOperationID: id, TraceID: traceID, ChargerOCPPIdentity: identity, OCPPConnectorNumber: connector, Kind: "TRIGGER_MESSAGE", Parameters: map[string]string{"requested_message": action}, State: "OCPP_CONFIRMED", OCPPResult: "Accepted", CompletedAt: &acceptedAt}
		if !accepted {
			store.operations[id].OCPPResult = "Rejected"
		}
		store.traces[traceID] = &V1Trace{TraceID: traceID, ChargerOCPPIdentity: identity, OCPPConnectorNumber: connector}
	}
	addExpectation("operation-heartbeat", "trace-heartbeat", "CP-1", "Heartbeat", 0, true)
	addExpectation("operation-meter", "trace-meter", "CP-1", "MeterValues", 2, true)
	addExpectation("operation-rejected", "trace-rejected", "CP-1", "Heartbeat", 0, false)

	lookup := func(identity, action string, connector int, scoped bool, observedAt time.Time) []V1TriggerMessageFollowOnExpectation {
		t.Helper()
		matches, err := store.ListV1TriggerMessageFollowOnExpectations(context.Background(), identity, action, connector, scoped, observedAt.Add(-time.Minute), observedAt)
		if err != nil {
			t.Fatal(err)
		}
		return matches
	}
	observedAt := acceptedAt.Add(time.Second)
	if matches := lookup("CP-1", "Heartbeat", 0, false, observedAt); len(matches) != 1 || matches[0].TraceID != "trace-heartbeat" {
		t.Fatalf("heartbeat matches=%#v", matches)
	}
	if matches := lookup("CP-1", "MeterValues", 1, true, observedAt); len(matches) != 0 {
		t.Fatalf("wrong connector matched=%#v", matches)
	}
	if matches := lookup("CP-1", "MeterValues", 2, true, observedAt); len(matches) != 1 || matches[0].TraceID != "trace-meter" {
		t.Fatalf("matching connector matches=%#v", matches)
	}
	if matches := lookup("CP-1", "StatusNotification", 2, true, observedAt); len(matches) != 0 {
		t.Fatalf("wrong action matched=%#v", matches)
	}
	if matches := lookup("CP-2", "Heartbeat", 0, false, observedAt); len(matches) != 0 {
		t.Fatalf("wrong charger matched=%#v", matches)
	}
	if matches := lookup("CP-1", "Heartbeat", 0, false, acceptedAt); len(matches) != 0 {
		t.Fatalf("pre-acceptance observation matched=%#v", matches)
	}
	if matches := lookup("CP-1", "Heartbeat", 0, false, acceptedAt.Add(time.Minute+time.Nanosecond)); len(matches) != 0 {
		t.Fatalf("post-deadline observation matched=%#v", matches)
	}
}

func TestV1TriggerMessageFollowOnOverlappingWindowsRemainNonUnique(t *testing.T) {
	acceptedAt := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	store := NewV1MemoryStore()
	for _, id := range []string{"one", "two"} {
		traceID := "trace-" + id
		store.operations[id] = &V1ChargerOperation{CMSOperationID: id, TraceID: traceID, ChargerOCPPIdentity: "CP-1", Kind: "TRIGGER_MESSAGE", Parameters: map[string]string{"requested_message": "Heartbeat"}, State: "OCPP_CONFIRMED", OCPPResult: "Accepted", CompletedAt: &acceptedAt}
		store.traces[traceID] = &V1Trace{TraceID: traceID, ChargerOCPPIdentity: "CP-1"}
	}
	matches, err := store.ListV1TriggerMessageFollowOnExpectations(context.Background(), "CP-1", "Heartbeat", 0, false, acceptedAt.Add(-time.Minute), acceptedAt.Add(time.Second))
	if err != nil || len(matches) != 2 {
		t.Fatalf("overlapping expectations=%#v err=%v", matches, err)
	}
}
