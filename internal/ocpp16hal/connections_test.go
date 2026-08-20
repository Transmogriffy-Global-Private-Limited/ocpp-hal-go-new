package ocpp16hal

import (
	"context"
	"errors"
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
	err        error
}

func (s *heartbeatRenewalStore) RenewCurrentV1ChargerConnection(_ context.Context, identity string, generation int64, at time.Time) error {
	s.calls++
	s.identity, s.generation, s.at = identity, generation, at
	return s.err
}

func TestHeartbeatAcknowledgesRefreshableLivenessFailure(t *testing.T) {
	store := &heartbeatRenewalStore{err: errors.New("database unavailable")}
	hal := New(state.NewRegistry(), store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	hal.connections.register("CP-1", "current", "127.0.0.1:1")
	confirmation, err := hal.OnHeartbeat("CP-1", nil)
	if err != nil || confirmation == nil || store.calls != 1 {
		t.Fatalf("confirmation=%#v error=%v calls=%d", confirmation, err, store.calls)
	}
}

type runtimeProjectionStore struct {
	store.V1Store
	calls int
	fail  bool
}

func (s *runtimeProjectionStore) RecordV1ChargerConnection(context.Context, string, int64, bool, time.Time) error {
	s.calls++
	if s.fail {
		s.fail = false
		return errors.New("database unavailable")
	}
	return nil
}

func TestPhysicalConnectionProjectionRetriesTheSameObservation(t *testing.T) {
	store := &runtimeProjectionStore{fail: true}
	hal := New(state.NewRegistry(), store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	observation := pendingRuntimeProjection{identity: "CP-1", generation: 1, online: false, observedAt: time.Now().UTC()}
	hal.persistRuntimeProjection(context.Background(), observation)
	hal.retryRuntimeProjections(context.Background())
	if store.calls != 2 {
		t.Fatalf("calls=%d", store.calls)
	}
	hal.runtimeMu.Lock()
	defer hal.runtimeMu.Unlock()
	if len(hal.pendingRuntime) != 0 {
		t.Fatalf("pending=%#v", hal.pendingRuntime)
	}
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
