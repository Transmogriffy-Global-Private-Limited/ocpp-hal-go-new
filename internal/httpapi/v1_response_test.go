package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/ocpp16hal"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

type v1ResponseTestStore struct {
	store.V1Store
	startCommand *store.V1RemoteCommand
	stopCommand  *store.V1RemoteCommand
}

func (s *v1ResponseTestStore) ValidateV1Mapping(context.Context, string, string, string, string, int) error {
	return nil
}

func (s *v1ResponseTestStore) CreateV1StartCommand(context.Context, store.V1StartCommandInput) (*store.V1RemoteCommand, bool, error) {
	return s.startCommand, true, nil
}

func (s *v1ResponseTestStore) ClaimV1StartDelivery(context.Context, string) (*store.V1RemoteCommand, bool, error) {
	return s.startCommand, false, nil
}

func (s *v1ResponseTestStore) CreateV1StopCommand(context.Context, store.V1StopCommandInput) (*store.V1RemoteCommand, bool, error) {
	return s.stopCommand, false, nil
}

func (s *v1ResponseTestStore) GetV1Command(_ context.Context, id string) (*store.V1RemoteCommand, error) {
	if s.startCommand != nil && s.startCommand.CMSCommandID == id {
		return s.startCommand, nil
	}
	return s.stopCommand, nil
}

func (s *v1ResponseTestStore) ClaimV1StopDelivery(context.Context, string) (*store.V1StopWorkflow, bool, error) {
	return nil, false, nil
}

func TestV1CommandHTTPResponsesUseCanonicalSnakeCase(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	transactionID := store.NewUUIDString()
	transactionNumber := int64(42)
	startID, stopID := store.NewUUIDString(), store.NewUUIDString()
	testStore := &v1ResponseTestStore{
		startCommand: &store.V1RemoteCommand{HALCommandID: store.NewUUIDString(), CMSCommandID: startID, Kind: "START", State: "OCPP_ACCEPTED", UpdatedAt: now},
		stopCommand:  &store.V1RemoteCommand{HALCommandID: store.NewUUIDString(), CMSCommandID: stopID, Kind: "STOP", State: "PERSISTED", HALTransactionID: transactionID, OCPPTransactionID: &transactionNumber, UpdatedAt: now},
	}
	h := ocpp16hal.New(state.NewRegistry(), testStore, slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler := NewServer(config.Config{V1CMSBearerToken: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)), h, testStore).Routes()

	start := map[string]any{"cms_command_id": startID, "cms_start_intent_id": store.NewUUIDString(), "cpo_id": store.NewUUIDString(), "customer_id": store.NewUUIDString(), "cms_charger_id": store.NewUUIDString(), "cms_connector_id": store.NewUUIDString(), "charger_ocpp_identity": "CP-test", "ocpp_connector_number": 1, "id_tag": "appv1_123456789012", "credential_expires_at": now.Add(time.Minute), "command_expires_at": now.Add(2 * time.Minute), "energy_limit_wh": 1000, "max_duration_seconds": 60}
	stop := map[string]any{"cms_command_id": stopID, "cms_charging_session_id": store.NewUUIDString(), "cpo_id": store.NewUUIDString(), "cms_charger_id": store.NewUUIDString(), "cms_connector_id": store.NewUUIDString(), "charger_ocpp_identity": "CP-test", "ocpp_connector_number": 1, "hal_transaction_id": transactionID, "ocpp_transaction_id": transactionNumber, "requested_stop_initiator": "CUSTOMER", "requested_stop_reason": "customer_requested", "command_expires_at": now.Add(time.Minute)}

	for _, request := range []struct {
		name, method, path string
		body               any
	}{
		{name: "start", method: http.MethodPost, path: "/v1/remote-commands/start", body: start},
		{name: "exact lookup", method: http.MethodGet, path: "/v1/remote-commands?cms_command_id=" + startID},
		{name: "stop", method: http.MethodPost, path: "/v1/remote-commands/stop", body: stop},
	} {
		t.Run(request.name, func(t *testing.T) {
			var body io.Reader
			if request.body != nil {
				raw, err := json.Marshal(request.body)
				if err != nil {
					t.Fatal(err)
				}
				body = bytes.NewReader(raw)
			}
			recorder := httptest.NewRecorder()
			httpRequest := httptest.NewRequest(request.method, request.path, body)
			httpRequest.Header.Set("Authorization", "Bearer test")
			if request.body != nil {
				httpRequest.Header.Set("Content-Type", "application/json")
				httpRequest.Header.Set("Idempotency-Key", map[string]any(request.body.(map[string]any))["cms_command_id"].(string))
				httpRequest.Header.Set("X-Correlation-ID", store.NewUUIDString())
			}
			handler.ServeHTTP(recorder, httpRequest)
			if recorder.Code != http.StatusAccepted && recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			assertCanonicalCommandJSON(t, recorder.Body.Bytes())
		})
	}
}

func TestV1ResponseViewsUseSnakeCaseAndExcludeStoreOnlyFields(t *testing.T) {
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	value := int64(7)
	views := []any{
		v1MappingView(&store.V1ChargerMapping{CPOID: "cpo", CMSChargerID: "charger", ChargerOCPPIdentity: "CP", Enabled: true, Connectors: []store.V1ConnectorMapping{{CPOID: "cpo", CMSChargerID: "charger", CMSConnectorID: "connector", ChargerOCPPIdentity: "CP", OCPPConnectorNumber: 1}}}),
		v1TransactionView(&store.V1Transaction{HALTransactionID: "transaction", CMSStartIntentID: "intent", CMSCommandID: "command", CPOID: "cpo", CustomerID: "customer-private", CMSChargerID: "charger", CMSConnectorID: "connector", ChargerOCPPIdentity: "CP", OCPPConnectorNumber: 1, IDTag: "appv1_private", OCPPTransactionID: 1, ActualStartedAt: now, MeterStartWh: 1, StopState: "NONE"}),
		v1StopWorkflowView(&store.V1StopWorkflow{HALTransactionID: "transaction", State: "PERSISTED", LastErrorDetail: "private detail", CreatedAt: now, UpdatedAt: now}),
		v1ChargerRuntimeView(&store.V1ChargerRuntime{CPOID: "cpo", CMSChargerID: "charger", ChargerOCPPIdentity: "CP", ConnectionState: "ONLINE", UpdatedAt: now, Connectors: []store.V1ConnectorRuntime{{CPOID: "cpo", CMSChargerID: "charger", CMSConnectorID: "connector", ChargerOCPPIdentity: "CP", OCPPConnectorNumber: 1, Status: "Available", UpdatedAt: now}}}),
		v1ConnectorRuntimeView(store.V1ConnectorRuntime{CPOID: "cpo", CMSChargerID: "charger", CMSConnectorID: "connector", ChargerOCPPIdentity: "CP", OCPPConnectorNumber: 1, Status: "Available", StatusSequence: value, UpdatedAt: now}),
	}
	for _, view := range views {
		raw, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"CPOID", "CustomerID", "IDTag", "LastErrorDetail", "HALCommandID"} {
			if bytes.Contains(raw, []byte(`"`+forbidden+`"`)) {
				t.Fatalf("%T leaked implicit/internal key %q in %s", view, forbidden, raw)
			}
		}
	}
}

func assertCanonicalCommandJSON(t *testing.T, raw []byte) {
	t.Helper()
	var body struct {
		Command map[string]json.RawMessage `json:"command"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"hal_command_id", "cms_command_id", "kind", "state", "hal_transaction_id", "ocpp_transaction_id", "updated_at"} {
		if _, ok := body.Command[key]; !ok {
			t.Fatalf("command missing canonical key %q: %s", key, raw)
		}
	}
	for _, key := range []string{"HALCommandID", "CMSCommandID", "HALTransactionID", "OCPPTransactionID", "UpdatedAt"} {
		if _, ok := body.Command[key]; ok {
			t.Fatalf("command exposes accidental Go key %q: %s", key, raw)
		}
	}
}
