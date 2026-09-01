package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/ocpp16hal"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

type traceHTTPStore struct {
	store.V1Store
	traces *store.V1MemoryStore
}

func (s *traceHTTPStore) EnsureV1Trace(ctx context.Context, value store.V1Trace) (*store.V1Trace, error) {
	return s.traces.EnsureV1Trace(ctx, value)
}
func (s *traceHTTPStore) BindV1TraceTransaction(ctx context.Context, id string, tx *store.V1Transaction) error {
	return s.traces.BindV1TraceTransaction(ctx, id, tx)
}
func (s *traceHTTPStore) EnsureV1TraceForTransaction(ctx context.Context, tx *store.V1Transaction) (*store.V1Trace, error) {
	return s.traces.EnsureV1TraceForTransaction(ctx, tx)
}
func (s *traceHTTPStore) FindV1TraceByTransaction(ctx context.Context, id string) (*store.V1Trace, error) {
	return s.traces.FindV1TraceByTransaction(ctx, id)
}
func (s *traceHTTPStore) FindV1TraceForConnector(ctx context.Context, identity string, connector int) (*store.V1Trace, error) {
	return s.traces.FindV1TraceForConnector(ctx, identity, connector)
}
func (s *traceHTTPStore) AppendV1TraceEvent(ctx context.Context, id string, input store.V1TraceEventInput) error {
	return s.traces.AppendV1TraceEvent(ctx, id, input)
}
func (s *traceHTTPStore) GetV1Trace(ctx context.Context, id string) (*store.V1Trace, error) {
	return s.traces.GetV1Trace(ctx, id)
}
func (s *traceHTTPStore) ListV1TraceEvents(ctx context.Context, id string, before time.Time, beforeID string, limit int) ([]store.V1TraceEvent, error) {
	return s.traces.ListV1TraceEvents(ctx, id, before, beforeID, limit)
}
func (s *traceHTTPStore) DeleteV1TracesBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	return s.traces.DeleteV1TracesBefore(ctx, before, limit)
}

func TestV1ChargingTraceIsPrivateAndReturnsSanitizedCursorEvidence(t *testing.T) {
	backend := store.NewV1MemoryStore()
	traceID := "11111111-1111-4111-8111-111111111111"
	if _, err := backend.EnsureV1Trace(context.Background(), store.V1Trace{TraceID: traceID, CPOID: "22222222-2222-4222-8222-222222222222", ChargerOCPPIdentity: "cp", OCPPConnectorNumber: 1}); err != nil {
		t.Fatal(err)
	}
	if err := backend.AppendV1TraceEvent(context.Background(), traceID, store.V1TraceEventInput{Source: "HAL", Target: "CMS", Category: "TEST", Protocol: "HTTP", Phase: "CHARGING", Summary: "safe", OccurredAt: time.Now().UTC(), Data: map[string]any{"meter_wh": int64(7), "id_tag": "private"}}); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	backendView := &traceHTTPStore{traces: backend}
	handler := NewServer(config.Config{V1CMSBearerToken: "test"}, logger, ocpp16hal.New(state.NewRegistry(), backendView, logger), backendView).Routes()
	for _, token := range []string{"", "test"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/v1/charging-traces/"+traceID+"/events?limit=1", nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		handler.ServeHTTP(recorder, request)
		if token == "" && recorder.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated status=%d", recorder.Code)
		}
		if token != "" {
			if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "id_tag") || !strings.Contains(recorder.Body.String(), "meter_wh") {
				t.Fatalf("trace response status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		}
	}
}
