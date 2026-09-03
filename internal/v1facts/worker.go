package v1facts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

type Worker struct {
	store  factDeliveryStore
	traces store.V1TraceStore
	client *http.Client
	url    string
	token  string
	logger *slog.Logger
}

type factDeliveryStore interface {
	ClaimV1Facts(context.Context, time.Time, int) ([]store.V1Fact, error)
	MarkV1FactDelivery(context.Context, string, string, int, bool, bool, string, time.Time) error
}

func New(cfg config.Config, v1Store store.V1Store, logger *slog.Logger) (*Worker, error) {
	if !cfg.V1FactDeliveryEnabled {
		return nil, nil
	}
	if strings.TrimSpace(cfg.V1CMSFactsURL) == "" || strings.TrimSpace(cfg.V1CMSFactsBearerToken) == "" {
		return nil, fmt.Errorf("HAL_V1_CMS_FACTS_URL and HAL_V1_CMS_FACT_BEARER_TOKEN are required when HAL_V1_FACT_DELIVERY_ENABLED=true")
	}
	worker := &Worker{store: v1Store, client: &http.Client{Timeout: 15 * time.Second}, url: cfg.V1CMSFactsURL, token: cfg.V1CMSFactsBearerToken, logger: logger}
	if traces, ok := v1Store.(store.V1TraceStore); ok {
		worker.traces = traces
	}
	return worker, nil
}

func (w *Worker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := w.RunOnce(ctx); err != nil {
			w.logger.Warn("v1 fact delivery pass failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) error {
	if w == nil {
		return nil
	}
	// One active delivery keeps the 30-second durable lease comfortably ahead
	// of the 15-second HTTP timeout. Parallelism can be raised only with an
	// explicit lease-budget review.
	facts, err := w.store.ClaimV1Facts(ctx, time.Now().UTC(), 1)
	if err != nil {
		return err
	}
	var deliveryErrors []error
	for _, fact := range facts {
		if err := w.deliver(ctx, fact); err != nil {
			deliveryErrors = append(deliveryErrors, err)
		}
	}
	return errors.Join(deliveryErrors...)
}

func (w *Worker) deliver(ctx context.Context, fact store.V1Fact) error {
	envelope, err := json.Marshal(fact.Envelope())
	if err != nil {
		return w.mark(ctx, fact, 0, false, true, "invalid durable fact payload", time.Now().UTC())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(envelope))
	if err != nil {
		return w.mark(ctx, fact, 0, false, true, "invalid fact destination", time.Now().UTC())
	}
	req.Header.Set("Authorization", "Bearer "+w.token)
	req.Header.Set("Idempotency-Key", fact.FactID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-ID", fact.FactID)
	response, err := w.client.Do(req)
	if err != nil {
		return w.mark(ctx, fact, 0, false, false, "transport failure", retryAt(fact.Retries))
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return w.mark(ctx, fact, response.StatusCode, true, false, "", time.Now().UTC())
	}
	receiverCode := readReceiverErrorCode(response.Body)
	if response.StatusCode == 429 || response.StatusCode == 500 || response.StatusCode == 502 || response.StatusCode == 503 || response.StatusCode == 504 {
		w.logReceiverResponse("v1 fact receiver returned a transient response", fact, response.StatusCode, receiverCode)
		return w.mark(ctx, fact, response.StatusCode, false, false, receiverDeliveryDetail(response.StatusCode, receiverCode), retryAt(fact.Retries))
	}
	w.logReceiverResponse("v1 fact receiver requires reconciliation", fact, response.StatusCode, receiverCode)
	return w.mark(ctx, fact, response.StatusCode, false, true, receiverDeliveryDetail(response.StatusCode, receiverCode), time.Now().UTC())
}

func (w *Worker) mark(ctx context.Context, fact store.V1Fact, statusCode int, success, terminal bool, detail string, next time.Time) error {
	if err := w.store.MarkV1FactDelivery(ctx, fact.FactID, fact.ClaimToken, statusCode, success, terminal, detail, next); err != nil {
		return fmt.Errorf("record fact delivery state for %s: %w", fact.FactID, err)
	}
	if success {
		w.recordDeliveredLifecycleFact(ctx, fact)
	}
	return nil
}

// recordDeliveredLifecycleFact records the service boundary only after CMS
// acknowledged the immutable fact and the durable outbox has been marked
// delivered. The trace event is best-effort evidence; it cannot affect fact
// retry, transaction, or settlement authority.
func (w *Worker) recordDeliveredLifecycleFact(ctx context.Context, fact store.V1Fact) {
	if w.traces == nil || (fact.FactType != "transaction.started" && fact.FactType != "transaction.completed") {
		return
	}
	var payload struct {
		HALTransactionID string `json:"hal_transaction_id"`
	}
	if json.Unmarshal(fact.Payload, &payload) != nil || payload.HALTransactionID == "" {
		return
	}
	trace, err := w.traces.FindV1TraceByTransaction(ctx, payload.HALTransactionID)
	if err != nil {
		return
	}
	phase := "STARTING"
	if fact.FactType == "transaction.completed" {
		phase = "POST_STOP"
	}
	if err := w.traces.AppendV1TraceEvent(ctx, trace.TraceID, store.V1TraceEventInput{Source: "HAL", Target: "CMS", Category: "FACT", Protocol: "HTTP", Phase: phase, Summary: fact.FactType + " fact delivered to CMS", OccurredAt: time.Now().UTC(), CorrelationID: fact.FactID, Data: map[string]any{"action": fact.FactType}}); err != nil && w.logger != nil {
		w.logger.Warn("failed to persist delivered fact diagnostic trace", "trace_id", trace.TraceID, "fact_id", fact.FactID, "error", err)
	}
}

func readReceiverErrorCode(body io.Reader) string {
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 4096)).Decode(&response); err != nil {
		return ""
	}
	code := strings.TrimSpace(response.Error.Code)
	if len(code) > 64 {
		return ""
	}
	for _, r := range code {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return ""
		}
	}
	return code
}

func receiverDeliveryDetail(status int, code string) string {
	if code == "" {
		return fmt.Sprintf("receiver HTTP %d", status)
	}
	return fmt.Sprintf("receiver HTTP %d (%s)", status, code)
}

func (w *Worker) logReceiverResponse(message string, fact store.V1Fact, status int, code string) {
	if w.logger != nil {
		w.logger.Warn(message, "fact_id", fact.FactID, "fact_type", fact.FactType, "status", status, "receiver_error_code", code)
	}
}

func retryAt(retries int) time.Time {
	delay := time.Second * time.Duration(1<<min(retries, 6))
	if delay > time.Minute {
		delay = time.Minute
	}
	return time.Now().UTC().Add(delay)
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
