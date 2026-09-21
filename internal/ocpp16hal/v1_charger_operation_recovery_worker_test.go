package ocpp16hal

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/state"
)

func TestV1ChargerOperationRecoveryWorkerSeparatesSlowPassesFromDeadlines(t *testing.T) {
	operations := &lifecycleRecoveryStoreSpy{}
	hal := New(state.NewRegistry(), operations, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		runV1ChargerOperationRecoveryWorker(ctx, time.Hour, func(context.Context) error {
			close(started)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}, nil)
	}()
	<-started

	for range 2 {
		deadlineDone := make(chan error, 1)
		go func() { deadlineDone <- hal.EnforceV1Deadlines(context.Background()) }()
		select {
		case err := <-deadlineDone:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("slow charger-operation recovery delayed deadline enforcement")
		}
	}
	deadlines, chargerOperations := operations.counts()
	if deadlines != 2 || chargerOperations != 0 {
		t.Fatalf("deadline passes=%d charger-operation passes=%d", deadlines, chargerOperations)
	}
	close(release)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("charger-operation recovery worker did not stop")
	}
}

func TestRunV1ChargerOperationRecoveryVisitsConnectedCharger(t *testing.T) {
	operations := &lifecycleRecoveryStoreSpy{chargerOperationListed: make(chan struct{}, 1)}
	hal := New(state.NewRegistry(), operations, slog.New(slog.NewTextHandler(io.Discard, nil)))
	hal.connections.register("charger-a", "key-a", "")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		hal.RunV1ChargerOperationRecovery(ctx)
	}()
	select {
	case <-operations.chargerOperationListed:
	case <-time.After(time.Second):
		t.Fatal("recovery worker did not visit connected charger")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("charger-operation recovery worker did not stop")
	}
}

func TestV1ChargerOperationRecoveryWorkerIsSequentialAndStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 2)
	workerDone := make(chan struct{})
	var active atomic.Int32
	var maxActive atomic.Int32
	var passes atomic.Int32
	go func() {
		defer close(workerDone)
		runV1ChargerOperationRecoveryWorker(ctx, time.Millisecond, func(ctx context.Context) error {
			current := active.Add(1)
			for {
				observed := maxActive.Load()
				if current <= observed || maxActive.CompareAndSwap(observed, current) {
					break
				}
			}
			passes.Add(1)
			started <- struct{}{}
			<-ctx.Done()
			active.Add(-1)
			return ctx.Err()
		}, nil)
	}()
	<-started
	time.Sleep(10 * time.Millisecond)
	if maxActive.Load() != 1 || passes.Load() != 1 {
		t.Fatalf("max active=%d passes=%d, want one blocked pass", maxActive.Load(), passes.Load())
	}
	cancel()
	select {
	case <-workerDone:
	case <-time.After(time.Second):
		t.Fatal("charger-operation recovery worker ignored cancellation")
	}
}
