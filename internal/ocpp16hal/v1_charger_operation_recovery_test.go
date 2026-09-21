package ocpp16hal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
)

var recoveryOperationSequence atomic.Uint64

type lifecycleRecoveryStoreSpy struct {
	store.V1Store
	recovered bool
}

func (s *lifecycleRecoveryStoreSpy) RecoverV1CommandDelivery(context.Context) error { return nil }
func (s *lifecycleRecoveryStoreSpy) RecoverV1StopDelivery(context.Context) error    { return nil }
func (s *lifecycleRecoveryStoreSpy) RecoverV1ChargerOperationDelivery(context.Context) error {
	s.recovered = true
	return nil
}
func (s *lifecycleRecoveryStoreSpy) ListV1DispatchableStops(context.Context, int) ([]*store.V1StopWorkflow, error) {
	return nil, nil
}
func (s *lifecycleRecoveryStoreSpy) ListV1OverdueTransactions(context.Context, time.Time, int) ([]*store.V1Transaction, error) {
	return nil, nil
}

type chargerOperationRecoveryDispatcher struct {
	mu       sync.Mutex
	calls    map[string]int
	failures map[string]error
}

func (d *chargerOperationRecoveryDispatcher) DispatchChargerOperation(_ context.Context, operation *store.V1ChargerOperation) (string, *core.GetConfigurationConfirmation, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls[operation.CMSOperationID]++
	return "Accepted", nil, d.failures[operation.CMSOperationID]
}

func (d *chargerOperationRecoveryDispatcher) count(id string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls[id]
}

func newRecoveryOperation(t *testing.T, operations *store.V1MemoryStore) *store.V1ChargerOperation {
	t.Helper()
	suffix := randomRecoverySuffix(t)
	op, duplicate, err := operations.CreateV1ChargerOperation(context.Background(), store.V1ChargerOperationInput{
		CMSOperationID:      "00000000-0000-4000-8000-" + suffix,
		RequestDigest:       "digest-" + suffix,
		CPOID:               "00000000-0000-4000-8000-000000000001",
		CMSChargerID:        "00000000-0000-4000-8000-000000000002",
		TraceID:             "00000000-0000-4000-8000-000000000003",
		ChargerOCPPIdentity: "recovery-" + suffix,
		Kind:                "CLEAR_CACHE",
	})
	if err != nil || duplicate {
		t.Fatalf("create operation duplicate=%t err=%v", duplicate, err)
	}
	return op
}

func randomRecoverySuffix(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%012x", recoveryOperationSequence.Add(1))
}

func TestV1ChargerOperationRecoverySafety(t *testing.T) {
	operations := store.NewV1MemoryStore()
	dispatcher := &chargerOperationRecoveryDispatcher{calls: map[string]int{}, failures: map[string]error{}}
	ctx := context.Background()

	t.Run("persisted operation dispatches exactly once after recovery", func(t *testing.T) {
		op := newRecoveryOperation(t, operations)
		for range 2 {
			if err := recoverDispatchableV1ChargerOperations(ctx, operations, dispatcher, "", func(*store.V1ChargerOperation) bool { return true }, nil); err != nil {
				t.Fatal(err)
			}
		}
		stored, err := operations.GetV1ChargerOperation(ctx, op.CMSOperationID)
		if err != nil || dispatcher.count(op.CMSOperationID) != 1 || stored.State != "OCPP_CONFIRMED" || stored.DeliveryAttempts != 1 {
			t.Fatalf("calls=%d operation=%#v err=%v", dispatcher.count(op.CMSOperationID), stored, err)
		}
	})

	t.Run("racing recovery actors have one physical dispatcher", func(t *testing.T) {
		op := newRecoveryOperation(t, operations)
		var group sync.WaitGroup
		for range 2 {
			group.Add(1)
			go func() {
				defer group.Done()
				if _, _, _, err := dispatchClaimedV1ChargerOperation(ctx, operations, dispatcher, op.CMSOperationID); err != nil {
					t.Error(err)
				}
			}()
		}
		group.Wait()
		if calls := dispatcher.count(op.CMSOperationID); calls != 1 {
			t.Fatalf("physical dispatches=%d, want 1", calls)
		}
	})

	t.Run("attempted and terminal rows never replay", func(t *testing.T) {
		attempted := newRecoveryOperation(t, operations)
		if _, claimed, err := operations.ClaimV1ChargerOperationDelivery(ctx, attempted.CMSOperationID); err != nil || !claimed {
			t.Fatalf("claim=%t err=%v", claimed, err)
		}
		if err := operations.RecoverV1ChargerOperationDelivery(ctx); err != nil {
			t.Fatal(err)
		}
		confirmed := newRecoveryOperation(t, operations)
		if _, _, _, err := dispatchClaimedV1ChargerOperation(ctx, operations, dispatcher, confirmed.CMSOperationID); err != nil {
			t.Fatal(err)
		}
		if err := recoverDispatchableV1ChargerOperations(ctx, operations, dispatcher, "", func(*store.V1ChargerOperation) bool { return true }, nil); err != nil {
			t.Fatal(err)
		}
		stored, err := operations.GetV1ChargerOperation(ctx, attempted.CMSOperationID)
		if err != nil || stored.State != "RECONCILIATION_REQUIRED" || dispatcher.count(attempted.CMSOperationID) != 0 || dispatcher.count(confirmed.CMSOperationID) != 1 {
			t.Fatalf("attempted=%#v calls=%d confirmed_calls=%d err=%v", stored, dispatcher.count(attempted.CMSOperationID), dispatcher.count(confirmed.CMSOperationID), err)
		}
	})

	t.Run("mixed batch isolates bad and offline operations", func(t *testing.T) {
		good, bad, offline := newRecoveryOperation(t, operations), newRecoveryOperation(t, operations), newRecoveryOperation(t, operations)
		dispatcher.failures[bad.CMSOperationID] = errors.New("result unavailable")
		if err := recoverDispatchableV1ChargerOperations(ctx, operations, dispatcher, "", func(operation *store.V1ChargerOperation) bool {
			return operation.CMSOperationID != offline.CMSOperationID
		}, nil); err != nil {
			t.Fatal(err)
		}
		goodStored, _ := operations.GetV1ChargerOperation(ctx, good.CMSOperationID)
		badStored, _ := operations.GetV1ChargerOperation(ctx, bad.CMSOperationID)
		offlineStored, _ := operations.GetV1ChargerOperation(ctx, offline.CMSOperationID)
		if goodStored.State != "OCPP_CONFIRMED" || badStored.State != "RECONCILIATION_REQUIRED" || offlineStored.State != "PERSISTED" || dispatcher.count(good.CMSOperationID) != 1 || dispatcher.count(bad.CMSOperationID) != 1 || dispatcher.count(offline.CMSOperationID) != 0 {
			t.Fatalf("good=%s bad=%s offline=%s calls=%d/%d/%d", goodStored.State, badStored.State, offlineStored.State, dispatcher.count(good.CMSOperationID), dispatcher.count(bad.CMSOperationID), dispatcher.count(offline.CMSOperationID))
		}
	})

	t.Run("connection-specific scan reaches its own queued operation", func(t *testing.T) {
		offline, connected := newRecoveryOperation(t, operations), newRecoveryOperation(t, operations)
		if err := recoverDispatchableV1ChargerOperations(ctx, operations, dispatcher, connected.ChargerOCPPIdentity, func(operation *store.V1ChargerOperation) bool {
			return operation.ChargerOCPPIdentity == connected.ChargerOCPPIdentity
		}, nil); err != nil {
			t.Fatal(err)
		}
		offlineStored, _ := operations.GetV1ChargerOperation(ctx, offline.CMSOperationID)
		connectedStored, _ := operations.GetV1ChargerOperation(ctx, connected.CMSOperationID)
		if offlineStored.State != "PERSISTED" || connectedStored.State != "OCPP_CONFIRMED" || dispatcher.count(connected.CMSOperationID) != 1 {
			t.Fatalf("offline=%s connected=%s connected_calls=%d", offlineStored.State, connectedStored.State, dispatcher.count(connected.CMSOperationID))
		}
	})
}

func TestRecoverV1LifecycleIncludesChargerOperationRecovery(t *testing.T) {
	operations := &lifecycleRecoveryStoreSpy{}
	hal := New(state.NewRegistry(), operations, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := hal.RecoverV1Lifecycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !operations.recovered {
		t.Fatal("startup lifecycle omitted charger-operation recovery")
	}
}
