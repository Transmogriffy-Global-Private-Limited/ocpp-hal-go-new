package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/ocpp16hal"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

func TestValidUUIDRequiresCanonicalNonzeroUUID(t *testing.T) {
	valid := store.NewUUIDString()
	for _, value := range []string{valid, "00000000-0000-0000-0000-000000000000", strings.ToUpper(valid), valid + "0", "not-a-uuid"} {
		want := value == valid
		if got := validUUID(value); got != want {
			t.Fatalf("validUUID(%q)=%v, want %v", value, got, want)
		}
	}
}

type requeueStore struct {
	store.V1Store
	factID, correlationID string
}

func (s *requeueStore) RequeueV1Fact(_ context.Context, factID, correlationID string) error {
	s.factID, s.correlationID = factID, correlationID
	return nil
}

func TestFactRequeueRequiresExactIdentityAndCallsStore(t *testing.T) {
	factID, correlationID := store.NewUUIDString(), store.NewUUIDString()
	backend := &requeueStore{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewServer(config.Config{V1CMSBearerToken: "test"}, logger, ocpp16hal.New(state.NewRegistry(), backend, logger), backend).Routes()
	request := httptest.NewRequest(http.MethodPost, "/v1/facts/"+factID+"/requeue", nil)
	request.Header.Set("Authorization", "Bearer test")
	request.Header.Set("X-Correlation-ID", correlationID)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || backend.factID != factID || backend.correlationID != correlationID {
		t.Fatalf("status=%d store=%#v", recorder.Code, backend)
	}
}

func TestDecodeJSONRejectsTrailingValues(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"value":"first"} {"value":"second"}`))
	recorder := httptest.NewRecorder()
	var body struct {
		Value string `json:"value"`
	}
	if decodeJSON(recorder, request, &body) {
		t.Fatal("decodeJSON accepted a second JSON value")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", recorder.Code)
	}
}
