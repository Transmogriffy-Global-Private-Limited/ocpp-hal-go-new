package ocpp16hal

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

// statusTraceStore supplies only the store behavior exercised by
// OnStatusNotification while retaining the real V1Store and V1TraceStore
// contracts at the handler boundary.
type statusTraceStore struct {
	store.V1Store
	store.V1TraceStore

	trace          *store.V1Trace
	transaction    *store.V1Transaction
	transactionErr error
	statuses       []store.V1ConnectorRuntime
	events         []store.V1TraceEventInput
}

func (s *statusTraceStore) RecordV1ConnectorStatus(_ context.Context, runtime store.V1ConnectorRuntime) error {
	s.statuses = append(s.statuses, runtime)
	return nil
}

func (s *statusTraceStore) FindV1TraceForConnector(_ context.Context, identity string, connector int) (*store.V1Trace, error) {
	if s.trace == nil || s.trace.ChargerOCPPIdentity != identity || s.trace.OCPPConnectorNumber != connector {
		return nil, store.ErrV1TransactionNotFound
	}
	return s.trace, nil
}

func (s *statusTraceStore) GetV1Transaction(_ context.Context, id string) (*store.V1Transaction, error) {
	if s.transactionErr != nil {
		return nil, s.transactionErr
	}
	if s.transaction == nil || s.transaction.HALTransactionID != id {
		return nil, store.ErrV1TransactionNotFound
	}
	return s.transaction, nil
}

func (s *statusTraceStore) AppendV1TraceEvent(_ context.Context, traceID string, input store.V1TraceEventInput) error {
	if s.trace == nil || s.trace.TraceID != traceID {
		return store.ErrV1TransactionNotFound
	}
	s.events = append(s.events, input)
	return nil
}

func TestOnStatusNotificationTracePhaseFollowsAssociatedTransactionCompletion(t *testing.T) {
	completedAt := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name, status, wantPhase string
		completed               bool
	}{
		{name: "active transaction ignores available status", status: string(core.ChargePointStatusAvailable), wantPhase: "CHARGING"},
		{name: "completed transaction finishing", status: string(core.ChargePointStatusFinishing), wantPhase: "POST_STOP", completed: true},
		{name: "completed transaction available", status: string(core.ChargePointStatusAvailable), wantPhase: "POST_STOP", completed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			transaction := &store.V1Transaction{HALTransactionID: "11111111-1111-4111-8111-111111111111"}
			if test.completed {
				transaction.CompletedAt = &completedAt
			}
			traces := &statusTraceStore{
				trace:       &store.V1Trace{TraceID: "22222222-2222-4222-8222-222222222222", HALTransactionID: transaction.HALTransactionID, ChargerOCPPIdentity: "CP-trace", OCPPConnectorNumber: 1},
				transaction: transaction,
			}
			registry := state.NewRegistry()
			h := New(registry, traces, slog.New(slog.NewTextHandler(io.Discard, nil)))

			confirmation, err := h.OnStatusNotification("CP-trace", &core.StatusNotificationRequest{ConnectorId: 1, Status: core.ChargePointStatus(test.status), ErrorCode: core.NoError})
			if err != nil || confirmation == nil {
				t.Fatalf("confirmation=%#v err=%v", confirmation, err)
			}
			if len(traces.statuses) != 1 || traces.statuses[0].Status != test.status || traces.statuses[0].OCPPConnectorNumber != 1 {
				t.Fatalf("durable connector status=%#v", traces.statuses)
			}
			if len(traces.events) != 1 {
				t.Fatalf("trace events=%#v", traces.events)
			}
			event := traces.events[0]
			if event.Phase != test.wantPhase || event.Summary != "Connector status: "+test.status {
				t.Fatalf("trace event phase=%q summary=%q", event.Phase, event.Summary)
			}
			data, ok := event.Data.(map[string]any)
			if !ok || data["status"] != test.status || data["connector_id"] != 1 {
				t.Fatalf("trace event data=%#v", event.Data)
			}
			charger, ok := registry.Snapshot("CP-trace")
			if !ok || charger.Connectors["1"].Status != test.status || charger.Connectors["1"].ErrorCode != string(core.NoError) {
				t.Fatalf("connector runtime=%#v ok=%v", charger, ok)
			}
		})
	}
}

func TestOnStatusNotificationTracePhaseUsesStartingFallbackOnlyForUnboundTrace(t *testing.T) {
	traces := &statusTraceStore{
		trace: &store.V1Trace{TraceID: "33333333-3333-4333-8333-333333333333", ChargerOCPPIdentity: "CP-unbound", OCPPConnectorNumber: 1},
	}
	h := New(state.NewRegistry(), traces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	confirmation, err := h.OnStatusNotification("CP-unbound", &core.StatusNotificationRequest{ConnectorId: 1, Status: core.ChargePointStatusFinishing, ErrorCode: core.NoError})
	if err != nil || confirmation == nil || len(traces.events) != 1 || traces.events[0].Phase != "STARTING" {
		t.Fatalf("confirmation=%#v err=%v events=%#v", confirmation, err, traces.events)
	}
}

func TestOnStatusNotificationSkipsTraceWhenBoundTransactionCannotBeRead(t *testing.T) {
	traces := &statusTraceStore{
		trace:          &store.V1Trace{TraceID: "44444444-4444-4444-8444-444444444444", HALTransactionID: "55555555-5555-4555-8555-555555555555", ChargerOCPPIdentity: "CP-missing", OCPPConnectorNumber: 1},
		transactionErr: store.ErrV1TransactionNotFound,
	}
	registry := state.NewRegistry()
	h := New(registry, traces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	confirmation, err := h.OnStatusNotification("CP-missing", &core.StatusNotificationRequest{ConnectorId: 1, Status: core.ChargePointStatusAvailable, ErrorCode: core.NoError})
	if err != nil || confirmation == nil || len(traces.events) != 0 || len(traces.statuses) != 1 {
		t.Fatalf("confirmation=%#v err=%v trace_events=%#v persisted_statuses=%#v", confirmation, err, traces.events, traces.statuses)
	}
	if charger, ok := registry.Snapshot("CP-missing"); !ok || charger.Connectors["1"].Status != string(core.ChargePointStatusAvailable) {
		t.Fatalf("connector runtime=%#v ok=%v", charger, ok)
	}
}

func TestAutomaticV1StopInitiatorExcludesManualStops(t *testing.T) {
	for _, test := range []struct {
		initiator string
		want      bool
	}{
		{initiator: "ENERGY_LIMIT", want: true},
		{initiator: "TIME_LIMIT", want: true},
		{initiator: "MONEY_LIMIT", want: true},
		{initiator: "WALLET_LIMIT", want: true},
		{initiator: "CUSTOMER", want: false},
		{initiator: "CPO", want: false},
	} {
		if got := automaticV1StopInitiator(test.initiator); got != test.want {
			t.Fatalf("automaticV1StopInitiator(%q)=%v want %v", test.initiator, got, test.want)
		}
	}
}
