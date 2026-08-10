package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	ocpp16 "github.com/lorenzodonini/ocpp-go/ocpp1.6"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"

	"github.com/Transmogriffy-Global-Private-Limited/OCPPHAL_Go/internal/config"
	"github.com/Transmogriffy-Global-Private-Limited/OCPPHAL_Go/internal/httpapi"
	"github.com/Transmogriffy-Global-Private-Limited/OCPPHAL_Go/internal/ocpp16hal"
	"github.com/Transmogriffy-Global-Private-Limited/OCPPHAL_Go/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/OCPPHAL_Go/internal/store"
)

type v1ChargePoint struct{ cp ocpp16.ChargePoint }

func (c *v1ChargePoint) OnChangeAvailability(*core.ChangeAvailabilityRequest) (*core.ChangeAvailabilityConfirmation, error) {
	return core.NewChangeAvailabilityConfirmation(core.AvailabilityStatusAccepted), nil
}
func (c *v1ChargePoint) OnChangeConfiguration(*core.ChangeConfigurationRequest) (*core.ChangeConfigurationConfirmation, error) {
	return core.NewChangeConfigurationConfirmation(core.ConfigurationStatusAccepted), nil
}
func (c *v1ChargePoint) OnClearCache(*core.ClearCacheRequest) (*core.ClearCacheConfirmation, error) {
	return core.NewClearCacheConfirmation(core.ClearCacheStatusAccepted), nil
}
func (c *v1ChargePoint) OnDataTransfer(*core.DataTransferRequest) (*core.DataTransferConfirmation, error) {
	return core.NewDataTransferConfirmation(core.DataTransferStatusAccepted), nil
}
func (c *v1ChargePoint) OnGetConfiguration(*core.GetConfigurationRequest) (*core.GetConfigurationConfirmation, error) {
	return core.NewGetConfigurationConfirmation([]core.ConfigurationKey{}), nil
}
func (c *v1ChargePoint) OnRemoteStopTransaction(*core.RemoteStopTransactionRequest) (*core.RemoteStopTransactionConfirmation, error) {
	return core.NewRemoteStopTransactionConfirmation(types.RemoteStartStopStatusAccepted), nil
}
func (c *v1ChargePoint) OnReset(*core.ResetRequest) (*core.ResetConfirmation, error) {
	return core.NewResetConfirmation(core.ResetStatusAccepted), nil
}
func (c *v1ChargePoint) OnUnlockConnector(*core.UnlockConnectorRequest) (*core.UnlockConnectorConfirmation, error) {
	return core.NewUnlockConnectorConfirmation(core.UnlockStatusUnlocked), nil
}

func (c *v1ChargePoint) OnRemoteStartTransaction(request *core.RemoteStartTransactionRequest) (*core.RemoteStartTransactionConfirmation, error) {
	go func() {
		_, _ = c.cp.Authorize(request.IdTag)
		connector := 1
		if request.ConnectorId != nil {
			connector = *request.ConnectorId
		}
		_, _ = c.cp.StartTransaction(connector, request.IdTag, 12000, types.Now())
		_, _ = c.cp.StatusNotification(connector, core.NoError, core.ChargePointStatusCharging)
	}()
	return core.NewRemoteStartTransactionConfirmation(types.RemoteStartStopStatusAccepted), nil
}

func TestV1PostgresHTTPToOCPPStart(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL OCPP integration test")
	}
	ctx := context.Background()
	v1Store, err := store.NewPostgresStore(config.Config{DatabaseURL: dsn})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	legacyStore := store.NewMemoryStore()
	hal := ocpp16hal.New(state.NewRegistry(), legacyStore, nil, logger)
	hal.SetV1Store(v1Store)
	port := freePort(t)
	go hal.Start(port, "/{ws}")
	defer hal.Stop()
	if err := waitForPort(port, 5*time.Second); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{V1Enabled: true, V1CMSBearerToken: "v1-test-service-token", RESTHost: "127.0.0.1", RESTPort: "0"}
	api := httptest.NewServer(httpapi.NewServer(cfg, logger, state.NewRegistry(), hal, legacyStore, store.NewTransactionUpdates(), v1Store).Routes())
	defer api.Close()
	clientID := "CP-V1-OCPP-" + store.NewUUIDString()[:8]
	chargePoint := &v1ChargePoint{}
	chargePoint.cp = ocpp16.NewChargePoint(clientID, nil, nil)
	chargePoint.cp.SetCoreHandler(chargePoint)
	if err := chargePoint.cp.Start(fmt.Sprintf("ws://127.0.0.1:%d", port)); err != nil {
		t.Fatal(err)
	}
	defer chargePoint.cp.Stop()
	deadline := time.Now().Add(8 * time.Second)
	for !chargePoint.cp.IsConnected() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !chargePoint.cp.IsConnected() {
		t.Fatal("virtual charger did not connect")
	}
	if _, err := chargePoint.cp.BootNotification("V1Test", "TransEV"); err != nil {
		t.Fatal(err)
	}
	if _, err := chargePoint.cp.StatusNotification(1, core.NoError, core.ChargePointStatusAvailable); err != nil {
		t.Fatal(err)
	}

	cpoID, chargerID, connectorID, commandID, intentID, customerID := store.NewUUIDString(), store.NewUUIDString(), store.NewUUIDString(), store.NewUUIDString(), store.NewUUIDString(), store.NewUUIDString()
	putJSON(t, api.URL+"/v1/mappings/chargers/"+chargerID, chargerID, map[string]any{"cpo_id": cpoID, "cms_charger_id": chargerID, "charger_ocpp_identity": clientID, "enabled": true, "connectors": []map[string]any{{"cms_connector_id": connectorID, "ocpp_connector_number": 1}}})
	now := time.Now().UTC()
	putJSON(t, api.URL+"/v1/remote-commands/start", commandID, map[string]any{"cms_command_id": commandID, "cms_start_intent_id": intentID, "cpo_id": cpoID, "customer_id": customerID, "cms_charger_id": chargerID, "cms_connector_id": connectorID, "charger_ocpp_identity": clientID, "ocpp_connector_number": 1, "id_tag": "appv1_" + store.NewUUIDString()[:12], "credential_expires_at": now.Add(time.Minute), "command_expires_at": now.Add(2 * time.Minute), "energy_limit_wh": 9000, "max_duration_seconds": 3600})
	deadline = time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		tx, err := v1Store.GetV1TransactionByStartIntent(ctx, intentID)
		if err == nil {
			if tx.OCPPTransactionID <= 0 || tx.MeterStartWh != 12000 || tx.StopDeadlineAt == nil {
				t.Fatalf("materialized transaction=%#v", tx)
			}
			command, err := v1Store.GetV1Command(ctx, commandID)
			if err != nil || command.State != "MATERIALIZED" {
				t.Fatalf("command=%#v err=%v", command, err)
			}
			runtime, err := v1Store.GetV1ChargerRuntime(ctx, clientID)
			if err == nil && runtime.ConnectionState == "ONLINE" && len(runtime.Connectors) == 1 && runtime.Connectors[0].Status == "Charging" {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("v1 transaction did not materialize")
}

func putJSON(t *testing.T, url, idempotency string, body any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains([]byte(url), []byte("/mappings/")) {
		req.Method = http.MethodPut
	}
	req.Header.Set("Authorization", "Bearer v1-test-service-token")
	req.Header.Set("Idempotency-Key", idempotency)
	req.Header.Set("X-Correlation-ID", "test-correlation")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("%s returned %s", url, resp.Status)
	}
}
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("OCPP listener on %d did not become ready", port)
}
