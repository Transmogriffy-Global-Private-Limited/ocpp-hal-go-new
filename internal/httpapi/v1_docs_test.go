package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/OCPPHAL_Go/internal/config"
	"github.com/Transmogriffy-Global-Private-Limited/OCPPHAL_Go/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/OCPPHAL_Go/internal/store"
)

func TestAPIDocsToggle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	enabled := httptest.NewServer(NewServer(config.Config{APIDocsEnabled: true}, logger, state.NewRegistry(), nil, store.NewMemoryStore(), store.NewTransactionUpdates()).Routes())
	defer enabled.Close()
	for _, path := range []string{"/openapi.json", "/docs"} {
		response, err := http.Get(enabled.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("enabled %s=%d", path, response.StatusCode)
		}
	}
	disabled := httptest.NewServer(NewServer(config.Config{}, logger, state.NewRegistry(), nil, store.NewMemoryStore(), store.NewTransactionUpdates()).Routes())
	defer disabled.Close()
	response, err := http.Get(disabled.URL + "/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("disabled docs=%d", response.StatusCode)
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(data []byte) (int, error) { w.t.Log(string(data)); return len(data), nil }
