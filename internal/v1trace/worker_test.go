package v1trace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

type traceMark struct {
	eventID  string
	status   int
	success  bool
	terminal bool
}

type fakeTraceDeliveryStore struct {
	mu         sync.Mutex
	deliveries []store.V1TraceDelivery
	marks      []traceMark
}

func (s *fakeTraceDeliveryStore) ClaimV1TraceDeliveries(context.Context, time.Time, int) ([]store.V1TraceDelivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]store.V1TraceDelivery(nil), s.deliveries...), nil
}

func (s *fakeTraceDeliveryStore) MarkV1TraceDelivery(_ context.Context, eventID, _ string, status int, success, terminal bool, _ string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.marks = append(s.marks, traceMark{eventID: eventID, status: status, success: success, terminal: terminal})
	return nil
}

func TestWorkerDeliversObjectDataAndUsesTraceEventIdempotency(t *testing.T) {
	var got map[string]any
	var authorization, idempotency string
	receiver := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		idempotency = request.Header.Get("Idempotency-Key")
		if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	delivery := testTraceDelivery()
	fake := &fakeTraceDeliveryStore{deliveries: []store.V1TraceDelivery{delivery}}
	worker := &Worker{store: fake, client: receiver.Client(), url: receiver.URL, token: "trace-token"}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer trace-token" || idempotency != delivery.Event.EventID {
		t.Fatalf("authorization=%q idempotency=%q", authorization, idempotency)
	}
	if data, ok := got["data"].(map[string]any); !ok || data["meter_wh"] != float64(1200) {
		t.Fatalf("data was not a JSON object: %#v", got["data"])
	}
	if len(fake.marks) != 1 || !fake.marks[0].success || fake.marks[0].terminal || fake.marks[0].eventID != delivery.Event.EventID {
		t.Fatalf("marks=%#v", fake.marks)
	}
}

func TestWorkerClassifiesRetryAndTerminalResponses(t *testing.T) {
	for _, testcase := range []struct {
		status   int
		terminal bool
	}{{http.StatusServiceUnavailable, false}, {http.StatusConflict, true}} {
		t.Run(http.StatusText(testcase.status), func(t *testing.T) {
			receiver := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(testcase.status) }))
			defer receiver.Close()
			fake := &fakeTraceDeliveryStore{deliveries: []store.V1TraceDelivery{testTraceDelivery()}}
			worker := &Worker{store: fake, client: receiver.Client(), url: receiver.URL, token: "trace-token"}
			if err := worker.RunOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(fake.marks) != 1 || fake.marks[0].success || fake.marks[0].terminal != testcase.terminal || fake.marks[0].status != testcase.status {
				t.Fatalf("marks=%#v", fake.marks)
			}
		})
	}
}

func TestWorkerUsesThePersistedImmutablePayload(t *testing.T) {
	var got map[string]any
	receiver := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	delivery := testTraceDelivery()
	delivery.Payload = map[string]any{
		"schema_version":           float64(1),
		"trace_id":                 delivery.Trace.TraceID,
		"event_id":                 delivery.Event.EventID,
		"cpo_id":                   delivery.Trace.CPOID,
		"hal_transaction_id":       nil,
		"ocpp_transaction_id":      nil,
		"charger_ocpp_identity":    delivery.Trace.ChargerOCPPIdentity,
		"ocpp_connector_number":    float64(1),
		"source":                   delivery.Event.Source,
		"target":                   delivery.Event.Target,
		"category":                 delivery.Event.Category,
		"protocol":                 delivery.Event.Protocol,
		"phase":                    delivery.Event.Phase,
		"summary":                  delivery.Event.Summary,
		"occurred_at":              delivery.Event.OccurredAt.Format(time.RFC3339Nano),
		"data":                     map[string]any{},
		"immutable_content_sha256": delivery.ContentSHA256,
	}
	changed := "different-transaction-after-event"
	delivery.Trace.HALTransactionID = changed
	fake := &fakeTraceDeliveryStore{deliveries: []store.V1TraceDelivery{delivery}}
	worker := &Worker{store: fake, client: receiver.Client(), url: receiver.URL, token: "trace-token"}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got["hal_transaction_id"] != nil || got["ocpp_transaction_id"] != nil {
		t.Fatalf("worker rebuilt a mutable trace root instead of using the persisted payload: %#v", got)
	}
}

func testTraceDelivery() store.V1TraceDelivery {
	ocpp := int64(654321)
	return store.V1TraceDelivery{
		SchemaVersion: 1,
		Trace:         store.V1Trace{TraceID: "2d2e4e72-859d-45c5-92dc-56fb9c7c2e25", CPOID: "02a62d77-bbe9-4d52-9bdf-1e3e4d82b4cf", HALTransactionID: "ba5f9fd8-c6e8-4c96-b5a6-ff6b6ef92d8b", OCPPTransactionID: &ocpp, ChargerOCPPIdentity: "CP-01", OCPPConnectorNumber: 1},
		Event:         store.V1TraceEvent{EventID: "eb604802-3a7b-4139-9ca3-8786c5bc5f9e", TraceID: "2d2e4e72-859d-45c5-92dc-56fb9c7c2e25", Source: "CHARGER", Target: "HAL", Category: "METER", Protocol: "OCPP1.6", Phase: "CHARGING", Summary: "Meter observed", OccurredAt: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC), Data: map[string]any{"meter_wh": int64(1200)}},
		ContentSHA256: "a3e8f6d90f48ab5026bba5b7e6f8dc9dd257a77a2b50d8ee45002c3a4f6d4f72",
		ClaimToken:    "038d97d0-7bd5-4d47-93fd-42b24522cfcc",
	}
}
