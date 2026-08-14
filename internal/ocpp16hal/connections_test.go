package ocpp16hal

import "testing"

func TestConnectionTrackerFencesSupersededDisconnect(t *testing.T) {
	tracker := newConnectionTracker()
	first, previous := tracker.register("CP-1", "first", "127.0.0.1:1")
	if first.Generation != 1 || previous != nil {
		t.Fatalf("first registration=%#v previous=%#v", first, previous)
	}
	second, previous := tracker.register("CP-1", "second", "127.0.0.1:2")
	if second.Generation != 2 || previous == nil || previous.Generation != first.Generation {
		t.Fatalf("second registration=%#v previous=%#v", second, previous)
	}

	current, active := tracker.unregisterIfCurrent("CP-1", "first")
	if current || active == nil || active.Generation != second.Generation {
		t.Fatalf("superseded disconnect current=%v active=%#v", current, active)
	}
	current, disconnected := tracker.unregisterIfCurrent("CP-1", "second")
	if !current || disconnected == nil || disconnected.Generation != second.Generation {
		t.Fatalf("current disconnect current=%v disconnected=%#v", current, disconnected)
	}
	current, disconnected = tracker.unregisterIfCurrent("CP-1", "second")
	if current || disconnected != nil {
		t.Fatalf("duplicate disconnect current=%v disconnected=%#v", current, disconnected)
	}
}

func TestConnectionTrackerStartsNewProcessAtGenerationOne(t *testing.T) {
	tracker := newConnectionTracker()
	connection, previous := tracker.register("CP-1", "first", "127.0.0.1:1")
	if connection.Generation != 1 || previous != nil {
		t.Fatalf("new process registration=%#v previous=%#v", connection, previous)
	}
}
