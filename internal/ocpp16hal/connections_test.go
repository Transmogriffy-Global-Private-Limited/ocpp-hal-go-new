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

type heartbeatRenewalStore struct {
	store.V1Store
	calls      int
	identity   string
	generation int64
	at         time.Time
}

func (s *heartbeatRenewalStore) RenewCurrentV1ChargerConnection(_ context.Context, identity string, generation int64, at time.Time) error {
	s.calls++
	s.identity, s.generation, s.at = identity, generation, at
	return nil
}

func TestHeartbeatRenewsOnlyCurrentTrackedConnection(t *testing.T) {
	store := &heartbeatRenewalStore{}
	hal := New(state.NewRegistry(), store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	connection, _ := hal.connections.register("CP-1", "current", "127.0.0.1:1")

	if _, err := hal.OnHeartbeat("CP-1", nil); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || store.identity != "CP-1" || store.generation != int64(connection.Generation) || store.at.IsZero() {
		t.Fatalf("heartbeat renewal=%#v", store)
	}

	if current, _ := hal.connections.unregisterIfCurrent("CP-1", "current"); !current {
		t.Fatal("current connection was not removed")
	}
	if _, err := hal.OnHeartbeat("CP-1", nil); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 {
		t.Fatalf("heartbeat without a current connection renewed %d times", store.calls)
	}
}
