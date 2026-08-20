package v1facts

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

type factDeliveryMark struct {
	factID     string
	statusCode int
	success    bool
	terminal   bool
}

type fakeFactDeliveryStore struct {
	mu      sync.Mutex
	facts   []store.V1Fact
	marks   []factDeliveryMark
	markErr error
}

func (s *fakeFactDeliveryStore) ClaimV1Facts(context.Context, time.Time, int) ([]store.V1Fact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]store.V1Fact(nil), s.facts...), nil
}

func (s *fakeFactDeliveryStore) MarkV1FactDelivery(_ context.Context, factID, _ string, statusCode int, success, terminal bool, _ string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.marks = append(s.marks, factDeliveryMark{factID: factID, statusCode: statusCode, success: success, terminal: terminal})
	return s.markErr
}

func TestWorkerClassifiesReceiverResponsesAndPreservesEnvelope(t *testing.T) {
	transient := []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout}
	terminal := []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusGone, http.StatusUnprocessableEntity}
	statuses := append([]int{http.StatusNoContent}, transient...)
	statuses = append(statuses, terminal...)
	for _, status := range statuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var body []byte
			var idempotency string
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body = append([]byte(nil), mustReadBody(t, r)...)
				idempotency = r.Header.Get("Idempotency-Key")
				w.WriteHeader(status)
			}))
			defer receiver.Close()
			fact := testFact()
			fake := &fakeFactDeliveryStore{facts: []store.V1Fact{fact}}
			worker := &Worker{store: fake, client: receiver.Client(), url: receiver.URL, token: "test-token"}
			if err := worker.RunOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(fake.marks) != 1 {
				t.Fatalf("marks=%#v", fake.marks)
			}
			mark := fake.marks[0]
			if mark.factID != fact.FactID || mark.statusCode != status {
				t.Fatalf("mark=%#v", mark)
			}
			if mark.success != (status == http.StatusNoContent) || mark.terminal != isTerminalStatus(status) {
				t.Fatalf("mark classification=%#v", mark)
			}
			if idempotency != fact.FactID || len(body) == 0 {
				t.Fatalf("idempotency=%q body=%s", idempotency, body)
			}
		})
	}
}

func TestWorkerRetriesSameImmutableFactAfterLostResponse(t *testing.T) {
	fact := testFact()
	var bodies [][]byte
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodies = append(bodies, append([]byte(nil), mustReadBody(t, r)...))
		if len(bodies) == 1 {
			hijacker := w.(http.Hijacker)
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = connection.Close()
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	fake := &fakeFactDeliveryStore{facts: []store.V1Fact{fact}}
	worker := &Worker{store: fake, client: receiver.Client(), url: receiver.URL, token: "test-token"}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || string(bodies[0]) != string(bodies[1]) {
		t.Fatalf("redelivered bodies=%q", bodies)
	}
	if len(fake.marks) != 2 || fake.marks[0].success || !fake.marks[1].success {
		t.Fatalf("marks=%#v", fake.marks)
	}
}

func TestWorkerRetriesTransportFailure(t *testing.T) {
	fake := &fakeFactDeliveryStore{facts: []store.V1Fact{testFact()}}
	worker := &Worker{store: fake, client: &http.Client{Timeout: time.Second}, url: "http://127.0.0.1:1", token: "test-token"}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.marks) != 1 || fake.marks[0].success || fake.marks[0].terminal {
		t.Fatalf("marks=%#v", fake.marks)
	}
}

func TestWorkerReportsDurableDeliveryStateWriteFailure(t *testing.T) {
	fake := &fakeFactDeliveryStore{facts: []store.V1Fact{testFact()}, markErr: errors.New("database unavailable")}
	worker := &Worker{store: fake, client: &http.Client{Timeout: time.Second}, url: "http://127.0.0.1:1", token: "test-token"}
	if err := worker.RunOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "record fact delivery state") {
		t.Fatalf("error=%v", err)
	}
}

func TestReceiverDeliveryDetailKeepsOnlyStableErrorCode(t *testing.T) {
	if got := receiverDeliveryDetail(http.StatusInternalServerError, "invalid_hal_fact"); got != "receiver HTTP 500 (invalid_hal_fact)" {
		t.Fatalf("detail=%q", got)
	}
	if got := readReceiverErrorCode(strings.NewReader(`{"error":{"code":"invalid_hal_fact","message":"must not be retained"}}`)); got != "invalid_hal_fact" {
		t.Fatalf("code=%q", got)
	}
	if got := readReceiverErrorCode(strings.NewReader(`{"error":{"code":"unsafe code"}}`)); got != "" {
		t.Fatalf("unsafe code=%q", got)
	}
}

func testFact() store.V1Fact {
	return store.V1Fact{FactID: "f8a8d975-1f5d-4971-b1c6-4d1c583675ef", ClaimToken: "e9b7a4ed-6106-4bd7-a209-0d08c888a2d1", FactType: "transaction.meter", SchemaVersion: 1, OccurredAt: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC), Producer: "ocpp-hal-go-new", ContentSHA256: "c3e72fd9d1f2f89e4ca9b1579dfdecc9e56a05a53240fa5d1d45907da16ac085", Payload: []byte(`{"meter_sequence":7,"meter_value_wh":106220}`)}
}

func isTerminalStatus(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusGone, http.StatusUnprocessableEntity:
		return true
	default:
		return false
	}
}

func mustReadBody(t *testing.T, request *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil && !errors.Is(err, http.ErrBodyReadAfterClose) {
		t.Fatal(err)
	}
	return body
}
