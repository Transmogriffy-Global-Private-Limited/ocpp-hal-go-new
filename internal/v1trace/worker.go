// Package v1trace owns only diagnostic trace delivery. It intentionally does
// not import v1facts or expose fact-delivery store methods.
package v1trace

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
	store      store.V1TraceDeliveryStore
	client     *http.Client
	url, token string
	logger     *slog.Logger
}

func New(cfg config.Config, traceStore store.V1TraceDeliveryStore, logger *slog.Logger) (*Worker, error) {
	if !cfg.V1TraceDeliveryEnabled {
		return nil, nil
	}
	if strings.TrimSpace(cfg.V1CMSTraceURL) == "" || strings.TrimSpace(cfg.V1CMSTraceBearerToken) == "" {
		return nil, fmt.Errorf("HAL_V1_CMS_TRACE_URL and HAL_V1_CMS_TRACE_BEARER_TOKEN are required when HAL_V1_TRACE_DELIVERY_ENABLED=true")
	}
	if traceStore == nil {
		return nil, errors.New("trace delivery store is required")
	}
	return &Worker{store: traceStore, client: &http.Client{Timeout: 15 * time.Second}, url: cfg.V1CMSTraceURL, token: cfg.V1CMSTraceBearerToken, logger: logger}, nil
}

func (w *Worker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := w.RunOnce(ctx); err != nil && w.logger != nil {
			w.logger.Warn("v1 trace delivery pass failed", "error", err)
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
	// Deliberately separate claim/lease and capacity from v1 facts. One event at
	// a time keeps the diagnostic worker bounded without starving fact delivery.
	deliveries, err := w.store.ClaimV1TraceDeliveries(ctx, time.Now().UTC(), 1)
	if err != nil {
		return err
	}
	var errs []error
	for _, delivery := range deliveries {
		if err := w.deliver(ctx, delivery); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (w *Worker) deliver(ctx context.Context, delivery store.V1TraceDelivery) error {
	body, err := json.Marshal(delivery.Envelope())
	if err != nil {
		return w.mark(ctx, delivery, 0, false, true, "invalid durable trace envelope", time.Now().UTC())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return w.mark(ctx, delivery, 0, false, true, "invalid trace destination", time.Now().UTC())
	}
	req.Header.Set("Authorization", "Bearer "+w.token)
	req.Header.Set("Idempotency-Key", delivery.Event.EventID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-ID", delivery.Event.EventID)
	response, err := w.client.Do(req)
	if err != nil {
		return w.mark(ctx, delivery, 0, false, false, "transport failure", retryAt(delivery.Retries))
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return w.mark(ctx, delivery, response.StatusCode, true, false, "", time.Now().UTC())
	}
	code := receiverCode(response.Body)
	if response.StatusCode == 429 || response.StatusCode == 500 || response.StatusCode == 502 || response.StatusCode == 503 || response.StatusCode == 504 {
		return w.mark(ctx, delivery, response.StatusCode, false, false, receiverDetail(response.StatusCode, code), retryAt(delivery.Retries))
	}
	return w.mark(ctx, delivery, response.StatusCode, false, true, receiverDetail(response.StatusCode, code), time.Now().UTC())
}

func (w *Worker) mark(ctx context.Context, delivery store.V1TraceDelivery, status int, success, terminal bool, detail string, next time.Time) error {
	return w.store.MarkV1TraceDelivery(ctx, delivery.Event.EventID, delivery.ClaimToken, status, success, terminal, detail, next)
}
func receiverCode(reader io.Reader) string {
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(reader, 4096)).Decode(&response) != nil {
		return ""
	}
	value := strings.TrimSpace(response.Error.Code)
	if len(value) > 64 {
		return ""
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return ""
		}
	}
	return value
}
func receiverDetail(status int, code string) string {
	if code == "" {
		return fmt.Sprintf("receiver HTTP %d", status)
	}
	return fmt.Sprintf("receiver HTTP %d (%s)", status, code)
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
