package v1facts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

type Worker struct {
	store  factDeliveryStore
	client *http.Client
	url    string
	token  string
	logger *slog.Logger
}

type factDeliveryStore interface {
	ClaimV1Facts(context.Context, time.Time, int) ([]store.V1Fact, error)
	MarkV1FactDelivery(context.Context, string, int, bool, bool, string, time.Time) error
}

func New(cfg config.Config, v1Store store.V1Store, logger *slog.Logger) (*Worker, error) {
	if !cfg.V1FactDeliveryEnabled {
		return nil, nil
	}
	if strings.TrimSpace(cfg.V1CMSFactsURL) == "" || strings.TrimSpace(cfg.V1CMSFactsBearerToken) == "" {
		return nil, fmt.Errorf("HAL_V1_CMS_FACTS_URL and HAL_V1_CMS_FACT_BEARER_TOKEN are required when HAL_V1_FACT_DELIVERY_ENABLED=true")
	}
	return &Worker{store: v1Store, client: &http.Client{Timeout: 15 * time.Second}, url: cfg.V1CMSFactsURL, token: cfg.V1CMSFactsBearerToken, logger: logger}, nil
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
	facts, err := w.store.ClaimV1Facts(ctx, time.Now().UTC(), 50)
	if err != nil {
		return err
	}
	for _, fact := range facts {
		w.deliver(ctx, fact)
	}
	return nil
}

func (w *Worker) deliver(ctx context.Context, fact store.V1Fact) {
	envelope, err := json.Marshal(fact.Envelope())
	if err != nil {
		_ = w.store.MarkV1FactDelivery(ctx, fact.FactID, 0, false, true, "invalid durable fact payload", time.Now().UTC())
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(envelope))
	if err != nil {
		_ = w.store.MarkV1FactDelivery(ctx, fact.FactID, 0, false, true, "invalid fact destination", time.Now().UTC())
		return
	}
	req.Header.Set("Authorization", "Bearer "+w.token)
	req.Header.Set("Idempotency-Key", fact.FactID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-ID", fact.FactID)
	response, err := w.client.Do(req)
	if err != nil {
		_ = w.store.MarkV1FactDelivery(ctx, fact.FactID, 0, false, false, "transport failure", retryAt(fact.Retries))
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		_ = w.store.MarkV1FactDelivery(ctx, fact.FactID, response.StatusCode, true, false, "", time.Now().UTC())
		return
	}
	if response.StatusCode == 429 || response.StatusCode == 500 || response.StatusCode == 502 || response.StatusCode == 503 || response.StatusCode == 504 {
		_ = w.store.MarkV1FactDelivery(ctx, fact.FactID, response.StatusCode, false, false, "transient receiver response", retryAt(fact.Retries))
		return
	}
	_ = w.store.MarkV1FactDelivery(ctx, fact.FactID, response.StatusCode, false, true, "receiver reconciliation required", time.Now().UTC())
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
