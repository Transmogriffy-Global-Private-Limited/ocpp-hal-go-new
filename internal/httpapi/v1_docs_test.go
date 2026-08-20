package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
)

func TestAPIDocsToggle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	enabled := httptest.NewServer(NewServer(config.Config{APIDocsEnabled: true}, logger, nil, nil).Routes())
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
	disabled := httptest.NewServer(NewServer(config.Config{}, logger, nil, nil).Routes())
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

func TestV1OpenAPICoversRegisteredRoutes(t *testing.T) {
	var document struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(v1OpenAPI, &document); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/v1/mappings/chargers/{cms_charger_id}",
		"/v1/remote-commands/start",
		"/v1/remote-commands/stop",
		"/v1/remote-commands",
		"/v1/transactions",
		"/v1/transactions/{hal_transaction_id}",
		"/v1/runtime/chargers/{charger_ocpp_identity}",
		"/v1/runtime/connectors/{cms_connector_id}",
	} {
		if _, ok := document.Paths[path]; !ok {
			t.Fatalf("OpenAPI is missing registered v1 route %s", path)
		}
	}
}

func TestV1OpenAPIRequiresCanonicalCommandResponse(t *testing.T) {
	var document struct {
		Components struct {
			Schemas map[string]struct {
				Required []string `json:"required"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(v1OpenAPI, &document); err != nil {
		t.Fatal(err)
	}
	command, ok := document.Components.Schemas["Command"]
	if !ok {
		t.Fatal("OpenAPI is missing Command response schema")
	}
	required := make(map[string]bool, len(command.Required))
	for _, field := range command.Required {
		required[field] = true
	}
	for _, field := range []string{"hal_command_id", "cms_command_id", "kind", "state", "hal_transaction_id", "ocpp_transaction_id", "updated_at"} {
		if !required[field] {
			t.Fatalf("Command schema does not require %q", field)
		}
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(data []byte) (int, error) { w.t.Log(string(data)); return len(data), nil }
