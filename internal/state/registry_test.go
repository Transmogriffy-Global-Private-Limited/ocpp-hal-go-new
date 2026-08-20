package state

import "testing"

func TestStatusAggregationRetainsAnyConnectorFault(t *testing.T) {
	registry := NewRegistry()
	registry.ApplyStatusNotification("CP-1", 1, "Faulted", "GroundFailure")
	registry.ApplyStatusNotification("CP-1", 2, "Available", "NoError")
	charger, ok := registry.Snapshot("CP-1")
	if !ok || !charger.HasError {
		t.Fatalf("charger=%#v found=%v", charger, ok)
	}
	registry.ApplyStatusNotification("CP-1", 1, "Available", "NoError")
	charger, _ = registry.Snapshot("CP-1")
	if charger.HasError {
		t.Fatalf("error persisted after all connector faults cleared: %#v", charger)
	}
}
