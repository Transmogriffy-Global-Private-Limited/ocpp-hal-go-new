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
	mu                     sync.Mutex
	recovered              bool
	deadlinePasses         int
	chargerOperationPasses int
	chargerOperationListed chan struct{}
}

func (s *lifecycleRecoveryStoreSpy) RecoverV1CommandDelivery(context.Context) error { return nil }
func (s *lifecycleRecoveryStoreSpy) RecoverV1StopDelivery(context.Context) error    { return nil }
func (s *lifecycleRecoveryStoreSpy) RecoverV1ChargerOperationDelivery(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recovered = true
	return nil
}
func (s *lifecycleRecoveryStoreSpy) ListV1DispatchableStops(context.Context, int) ([]*store.V1StopWorkflow, error) {
	return nil, nil
}
func (s *lifecycleRecoveryStoreSpy) ListV1OverdueTransactions(context.Context, time.Time, int) ([]*store.V1Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deadlinePasses++
	return nil, nil
}
func (s *lifecycleRecoveryStoreSpy) ListV1DispatchableChargerOperations(context.Context, string, int) ([]*store.V1ChargerOperation, error) {
	s.mu.Lock()
	s.chargerOperationPasses++
	listed := s.chargerOperationListed
	s.mu.Unlock()
	if listed != nil {
		select {
		case listed <- struct{}{}:
		default:
		}
	}
	return nil, nil
}

func (s *lifecycleRecoveryStoreSpy) counts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deadlinePasses, s.chargerOperationPasses
}

type chargerOperationRecoveryDispatcher struct {
	mu       sync.Mutex
	calls    map[string]int
	failures map[string]error
	after    func()
}

func (d *chargerOperationRecoveryDispatcher) DispatchChargerOperation(_ context.Context, operation *store.V1ChargerOperation) (string, *core.GetConfigurationConfirmation, error) {
	d.mu.Lock()
	d.calls[operation.CMSOperationID]++
	after, failure := d.after, d.failures[operation.CMSOperationID]
	d.mu.Unlock()
	if after != nil {
		after()
	}
	return "Accepted", nil, failure
}

func (d *chargerOperationRecoveryDispatcher) count(id string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls[id]
}

func newRecoveryOperation(t *testing.T, operations *store.V1MemoryStore) *store.V1ChargerOperation {
	return newRecoveryOperationForCharger(t, operations, "")
}

func newRecoveryOperationForCharger(t *testing.T, operations *store.V1MemoryStore, chargerOCPPIdentity string) *store.V1ChargerOperation {
	t.Helper()
	suffix := randomRecoverySuffix(t)
	if chargerOCPPIdentity == "" {
		chargerOCPPIdentity = "recovery-" + suffix
	}
	op, duplicate, err := operations.CreateV1ChargerOperation(context.Background(), store.V1ChargerOperationInput{
		CMSOperationID:      "00000000-0000-4000-8000-" + suffix,
		RequestDigest:       "digest-" + suffix,
		CPOID:               "00000000-0000-4000-8000-000000000001",
		CMSChargerID:        "00000000-0000-4000-8000-000000000002",
		TraceID:             "00000000-0000-4000-8000-000000000003",
		ChargerOCPPIdentity: chargerOCPPIdentity,
		Kind:                "CLEAR_CACHE",
	})
	if err != nil || duplicate {
		t.Fatalf("create operation duplicate=%t err=%v", duplicate, err)
	}
	return op
}

type contextAwareOperationStore struct {
	*store.V1MemoryStore
}

func (s *contextAwareOperationStore) MarkV1ChargerOperationDelivery(ctx context.Context, id, state, result, category string) (*store.V1ChargerOperation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.V1MemoryStore.MarkV1ChargerOperationDelivery(ctx, id, state, result, category)
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
			if err := recoverDispatchableV1ChargerOperations(ctx, ctx, operations, dispatcher, "", func(*store.V1ChargerOperation) bool { return true }, nil); err != nil {
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
				if _, _, _, err := dispatchClaimedV1ChargerOperation(ctx, ctx, operations, dispatcher, op.CMSOperationID); err != nil {
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
		if _, _, _, err := dispatchClaimedV1ChargerOperation(ctx, ctx, operations, dispatcher, confirmed.CMSOperationID); err != nil {
			t.Fatal(err)
		}
		if err := recoverDispatchableV1ChargerOperations(ctx, ctx, operations, dispatcher, "", func(*store.V1ChargerOperation) bool { return true }, nil); err != nil {
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
		if err := recoverDispatchableV1ChargerOperations(ctx, ctx, operations, dispatcher, "", func(operation *store.V1ChargerOperation) bool {
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
		if err := recoverDispatchableV1ChargerOperations(ctx, ctx, operations, dispatcher, connected.ChargerOCPPIdentity, func(operation *store.V1ChargerOperation) bool {
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

func TestV1ChargerOperationFinalizationSurvivesRequestCancellation(t *testing.T) {
	memory := store.NewV1MemoryStore()
	operations := &contextAwareOperationStore{V1MemoryStore: memory}
	op := newRecoveryOperation(t, memory)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	dispatcher := &chargerOperationRecoveryDispatcher{calls: map[string]int{}, failures: map[string]error{}, after: cancelRequest}

	completed, _, claimed, err := dispatchClaimedV1ChargerOperation(requestCtx, context.Background(), operations, dispatcher, op.CMSOperationID)
	if err != nil || !claimed || completed.State != "OCPP_CONFIRMED" || dispatcher.count(op.CMSOperationID) != 1 {
		t.Fatalf("completed=%#v claimed=%t calls=%d err=%v", completed, claimed, dispatcher.count(op.CMSOperationID), err)
	}
}

func TestV1ChargerOperationFinalizationFailureNeverReplays(t *testing.T) {
	memory := store.NewV1MemoryStore()
	operations := &contextAwareOperationStore{V1MemoryStore: memory}
	op := newRecoveryOperation(t, memory)
	dispatcher := &chargerOperationRecoveryDispatcher{calls: map[string]int{}, failures: map[string]error{}}
	stopped, stop := context.WithCancel(context.Background())
	stop()

	if _, _, claimed, err := dispatchClaimedV1ChargerOperation(context.Background(), stopped, operations, dispatcher, op.CMSOperationID); !claimed || !errors.Is(err, context.Canceled) {
		t.Fatalf("claimed=%t err=%v", claimed, err)
	}
	stored, err := memory.GetV1ChargerOperation(context.Background(), op.CMSOperationID)
	if err != nil || stored.State != "DELIVERY_ATTEMPTED" {
		t.Fatalf("operation=%#v err=%v", stored, err)
	}
	if err := recoverDispatchableV1ChargerOperations(context.Background(), context.Background(), operations, dispatcher, "", func(*store.V1ChargerOperation) bool { return true }, nil); err != nil {
		t.Fatal(err)
	}
	if calls := dispatcher.count(op.CMSOperationID); calls != 1 {
		t.Fatalf("physical dispatches=%d, want 1", calls)
	}
}

func TestV1ChargerOperationRecoveryDrainsBoundedBatches(t *testing.T) {
	operations := store.NewV1MemoryStore()
	dispatcher := &chargerOperationRecoveryDispatcher{calls: map[string]int{}, failures: map[string]error{}}
	const identity = "connected-batch"
	created := make([]*store.V1ChargerOperation, 0, v1ChargerOperationRecoveryBatch+1)
	for range v1ChargerOperationRecoveryBatch + 1 {
		created = append(created, newRecoveryOperationForCharger(t, operations, identity))
	}
	for range 2 {
		if err := recoverDispatchableV1ChargerOperations(context.Background(), context.Background(), operations, dispatcher, identity, func(*store.V1ChargerOperation) bool { return true }, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, operation := range created {
		stored, err := operations.GetV1ChargerOperation(context.Background(), operation.CMSOperationID)
		if err != nil || stored.State != "OCPP_CONFIRMED" || dispatcher.count(operation.CMSOperationID) != 1 {
			t.Fatalf("operation=%#v calls=%d err=%v", stored, dispatcher.count(operation.CMSOperationID), err)
		}
	}
}

func TestV1ChargerOperationRecoveryLeavesOfflineRowsUntilConnected(t *testing.T) {
	operations := store.NewV1MemoryStore()
	dispatcher := &chargerOperationRecoveryDispatcher{calls: map[string]int{}, failures: map[string]error{}}
	op := newRecoveryOperationForCharger(t, operations, "offline-then-connected")
	if err := recoverDispatchableV1ChargerOperations(context.Background(), context.Background(), operations, dispatcher, op.ChargerOCPPIdentity, func(*store.V1ChargerOperation) bool { return false }, nil); err != nil {
		t.Fatal(err)
	}
	stored, err := operations.GetV1ChargerOperation(context.Background(), op.CMSOperationID)
	if err != nil || stored.State != "PERSISTED" || dispatcher.count(op.CMSOperationID) != 0 {
		t.Fatalf("offline operation=%#v calls=%d err=%v", stored, dispatcher.count(op.CMSOperationID), err)
	}
	if err := recoverDispatchableV1ChargerOperations(context.Background(), context.Background(), operations, dispatcher, op.ChargerOCPPIdentity, func(*store.V1ChargerOperation) bool { return true }, nil); err != nil {
		t.Fatal(err)
	}
	stored, err = operations.GetV1ChargerOperation(context.Background(), op.CMSOperationID)
	if err != nil || stored.State != "OCPP_CONFIRMED" || dispatcher.count(op.CMSOperationID) != 1 {
		t.Fatalf("connected operation=%#v calls=%d err=%v", stored, dispatcher.count(op.CMSOperationID), err)
	}
}

func TestV1ChargerOperationRecoveryRespectsCancellation(t *testing.T) {
	operations := store.NewV1MemoryStore()
	dispatcher := &chargerOperationRecoveryDispatcher{calls: map[string]int{}, failures: map[string]error{}}
	op := newRecoveryOperation(t, operations)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := recoverDispatchableV1ChargerOperations(ctx, context.Background(), operations, dispatcher, "", func(*store.V1ChargerOperation) bool { return true }, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want cancellation", err)
	}
	if calls := dispatcher.count(op.CMSOperationID); calls != 0 {
		t.Fatalf("physical dispatches=%d, want 0", calls)
	}
}

func TestV1ChargerOperationRecoveryRotatesActiveConnections(t *testing.T) {
	hal := New(state.NewRegistry(), &lifecycleRecoveryStoreSpy{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	hal.connections.register("charger-b", "key-b", "")
	hal.connections.register("charger-a", "key-a", "")
	want := []string{"charger-a", "charger-b", "charger-a"}
	for _, expected := range want {
		identity, ok := hal.nextV1ChargerOperationRecoveryIdentity()
		if !ok || identity != expected {
			t.Fatalf("identity=%q ok=%t, want %q", identity, ok, expected)
		}
	}
}

func TestEnforceV1DeadlinesDoesNotRecoverChargerOperations(t *testing.T) {
	operations := &lifecycleRecoveryStoreSpy{}
	hal := New(state.NewRegistry(), operations, slog.New(slog.NewTextHandler(io.Discard, nil)))
	hal.connections.register("charger-a", "key-a", "")
	if err := hal.EnforceV1Deadlines(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadlines, chargerOperations := operations.counts()
	if deadlines != 1 || chargerOperations != 0 {
		t.Fatalf("deadline passes=%d charger-operation passes=%d", deadlines, chargerOperations)
	}
}

func TestConnectionAndPeriodicRecoveryRaceHasOnePhysicalDispatch(t *testing.T) {
	operations := store.NewV1MemoryStore()
	dispatcher := &chargerOperationRecoveryDispatcher{calls: map[string]int{}, failures: map[string]error{}}
	op := newRecoveryOperationForCharger(t, operations, "connected-race")
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := recoverDispatchableV1ChargerOperations(context.Background(), context.Background(), operations, dispatcher, op.ChargerOCPPIdentity, func(*store.V1ChargerOperation) bool { return true }, nil); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if calls := dispatcher.count(op.CMSOperationID); calls != 1 {
		t.Fatalf("physical dispatches=%d, want 1", calls)
	}
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
