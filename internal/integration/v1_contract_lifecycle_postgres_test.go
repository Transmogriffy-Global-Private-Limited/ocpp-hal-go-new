package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	ocpp16 "github.com/lorenzodonini/ocpp-go/ocpp1.6"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/httpapi"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/ocpp16hal"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/v1facts"
)

func TestV1ManualStopContractLifecycleAndFactDelivery(t *testing.T) {
	if os.Getenv("HAL_RUN_CONTRACT_LIFECYCLE") != "true" {
		t.Skip("HAL_RUN_CONTRACT_LIFECYCLE=true is required because this test consumes the shared PostgreSQL fact outbox")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL contract lifecycle test")
	}

	receiver := newFactReceiver(t, "fact-receiver-token")
	defer receiver.Close()

	ctx := context.Background()
	postgres, err := store.NewPostgresStore(config.Config{DatabaseURL: os.Getenv("DATABASE_URL")})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hal := ocpp16hal.New(state.NewRegistry(), postgres, logger)
	port := freePort(t)
	go hal.Start(port, "/{ws}")
	defer hal.Stop()
	if err := waitForPort(port, 5*time.Second); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		V1CMSBearerToken:      "cms-service-token",
		V1FactDeliveryEnabled: true,
		V1CMSFactsURL:         receiver.URL + "/v1/hal-facts",
		V1CMSFactsBearerToken: "fact-receiver-token",
	}
	api := httptest.NewServer(httpapi.NewServer(cfg, logger, hal, postgres).Routes())
	defer api.Close()

	ids := newContractIDs()
	response := contractRequest(t, http.MethodPut, api.URL+"/v1/mappings/chargers/"+ids.ChargerID, "cms-service-token", ids.ChargerID, map[string]any{
		"cpo_id": ids.CPOID, "cms_charger_id": ids.ChargerID, "charger_ocpp_identity": ids.Identity, "enabled": true,
		"connectors": []map[string]any{{"cms_connector_id": ids.ConnectorID, "ocpp_connector_number": 1}},
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("mapping status=%d", response.StatusCode)
	}
	response.Body.Close()

	cp := newContractChargePoint(t, ids.Identity)
	cp.connect(t, port)
	defer cp.stop()

	now := time.Now().UTC()
	start := map[string]any{
		"cms_command_id": ids.StartCommandID, "cms_start_intent_id": ids.StartIntentID,
		"cpo_id": ids.CPOID, "customer_id": ids.CustomerID, "cms_charger_id": ids.ChargerID,
		"cms_connector_id": ids.ConnectorID, "charger_ocpp_identity": ids.Identity,
		"ocpp_connector_number": 1, "id_tag": ids.IDTag,
		"credential_expires_at": now.Add(time.Minute), "command_expires_at": now.Add(2 * time.Minute),
		"energy_limit_wh": 5000, "max_duration_seconds": 3600,
	}
	response = contractRequest(t, http.MethodPost, api.URL+"/v1/remote-commands/start", "cms-service-token", ids.StartCommandID, start)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("start status=%d", response.StatusCode)
	}
	response.Body.Close()
	changedStart := make(map[string]any, len(start))
	for key, value := range start {
		changedStart[key] = value
	}
	changedStart["energy_limit_wh"] = int64(5001)
	response = contractRequest(t, http.MethodPost, api.URL+"/v1/remote-commands/start", "cms-service-token", ids.StartCommandID, changedStart)
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("changed start command status=%d", response.StatusCode)
	}
	response.Body.Close()

	// OCPP command acceptance is intentionally not transaction truth.
	if _, err := postgres.GetV1TransactionByStartIntent(ctx, ids.StartIntentID); err == nil {
		t.Fatal("RemoteStart acknowledgement materialized a transaction before StartTransaction")
	}

	cp.beginStart(t, ids.IDTag, 12000)
	transaction := waitForTransaction(t, postgres, ids.StartIntentID)
	if transaction.OCPPTransactionID <= 0 || transaction.MeterStartWh != 12000 {
		t.Fatalf("unexpected materialized transaction: %#v", transaction)
	}

	cp.meter(t, transaction.OCPPTransactionID, 12120)
	cp.meter(t, transaction.OCPPTransactionID, 12340)
	transaction = waitForMeter(t, postgres, transaction.HALTransactionID, 2)
	if transaction.LatestMeterWh == nil || *transaction.LatestMeterWh != 12340 || transaction.ConsumedWh == nil || *transaction.ConsumedWh != 340 {
		t.Fatalf("unexpected meter projection: %#v", transaction)
	}

	stop := map[string]any{
		"cms_command_id": ids.StopCommandID, "cms_charging_session_id": ids.SessionID,
		"cpo_id": ids.CPOID, "customer_id": ids.CustomerID, "cms_charger_id": ids.ChargerID,
		"cms_connector_id": ids.ConnectorID, "charger_ocpp_identity": ids.Identity,
		"ocpp_connector_number": 1, "hal_transaction_id": transaction.HALTransactionID,
		"ocpp_transaction_id": transaction.OCPPTransactionID, "requested_stop_initiator": "CUSTOMER",
		"requested_stop_reason": "user_requested", "command_expires_at": now.Add(2 * time.Minute),
	}
	response = contractRequest(t, http.MethodPost, api.URL+"/v1/remote-commands/stop", "cms-service-token", ids.StopCommandID, stop)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("stop status=%d", response.StatusCode)
	}
	response.Body.Close()
	workflow := waitForWorkflow(t, postgres, transaction.HALTransactionID, "OCPP_ACCEPTED")
	if workflow.DeliveryAttempts != 1 || workflow.RequestedStopInitiator != "CUSTOMER" {
		t.Fatalf("unexpected stop workflow: %#v", workflow)
	}
	if current, err := postgres.GetV1Transaction(ctx, transaction.HALTransactionID); err != nil || current.CompletedAt != nil {
		t.Fatalf("RemoteStop acknowledgement completed transaction: tx=%#v err=%v", current, err)
	}

	cp.stopTransaction(t, transaction.OCPPTransactionID, 12420, core.ReasonRemote)
	completed := waitForCompletion(t, postgres, transaction.HALTransactionID)
	if completed.MeterStopWh == nil || *completed.MeterStopWh != 12420 || completed.OCPPStopReason != string(core.ReasonRemote) {
		t.Fatalf("unexpected completion: %#v", completed)
	}

	worker, err := v1facts.New(cfg, postgres, logger)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if err := worker.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	receiver.requireTypes(t,
		"charger.connection.updated", "connector.status.updated", "transaction.started",
		"transaction.meter", "transaction.completed", "command.updated",
	)

	for _, path := range []string{
		"/v1/remote-commands?cms_command_id=" + ids.StartCommandID,
		"/v1/transactions?cms_start_intent_id=" + ids.StartIntentID,
		"/v1/transactions/" + transaction.HALTransactionID,
		"/v1/runtime/chargers/" + ids.Identity,
		"/v1/runtime/connectors/" + ids.ConnectorID,
	} {
		response = contractRequest(t, http.MethodGet, api.URL+path, "cms-service-token", "", nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("reconciliation %s status=%d", path, response.StatusCode)
		}
		response.Body.Close()
	}

	response = contractRequest(t, http.MethodGet, api.URL+"/v1/transactions/"+transaction.HALTransactionID, "wrong-token", "", nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized lookup status=%d", response.StatusCode)
	}
	response.Body.Close()
}

type contractIDs struct {
	CPOID, ChargerID, ConnectorID, StartCommandID, StartIntentID, CustomerID, StopCommandID, SessionID, Identity, IDTag string
}

func newContractIDs() contractIDs {
	return contractIDs{
		CPOID: store.NewUUIDString(), ChargerID: store.NewUUIDString(), ConnectorID: store.NewUUIDString(),
		StartCommandID: store.NewUUIDString(), StartIntentID: store.NewUUIDString(), CustomerID: store.NewUUIDString(),
		StopCommandID: store.NewUUIDString(), SessionID: store.NewUUIDString(),
		Identity: "CP-V1-CONTRACT-" + store.NewUUIDString()[:8], IDTag: "appv1_" + store.NewUUIDString()[:12],
	}
}

func contractRequest(t *testing.T, method, url, token, idempotency string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if idempotency != "" {
		request.Header.Set("Idempotency-Key", idempotency)
		request.Header.Set("X-Correlation-ID", store.NewUUIDString())
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

type contractChargePoint struct {
	cp       ocpp16.ChargePoint
	identity string
}

func newContractChargePoint(t *testing.T, identity string) *contractChargePoint {
	t.Helper()
	chargePoint := &contractChargePoint{identity: identity}
	chargePoint.cp = ocpp16.NewChargePoint(identity, nil, nil)
	chargePoint.cp.SetCoreHandler(chargePoint)
	return chargePoint
}

func (c *contractChargePoint) connect(t *testing.T, port int) {
	t.Helper()
	if err := c.cp.Start(fmt.Sprintf("ws://127.0.0.1:%d", port)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !c.cp.IsConnected() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !c.cp.IsConnected() {
		t.Fatal("virtual charge point did not connect")
	}
	if _, err := c.cp.BootNotification("V1ContractTest", "TransEV"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.cp.StatusNotification(1, core.NoError, core.ChargePointStatusAvailable); err != nil {
		t.Fatal(err)
	}
}

func (c *contractChargePoint) stop() { c.cp.Stop() }

func (c *contractChargePoint) beginStart(t *testing.T, idTag string, meterStart int) {
	t.Helper()
	confirmation, err := c.cp.Authorize(idTag)
	if err != nil || confirmation.IdTagInfo.Status != types.AuthorizationStatusAccepted {
		t.Fatalf("Authorize confirmation=%#v err=%v", confirmation, err)
	}
	startConfirmation, err := c.cp.StartTransaction(1, idTag, meterStart, types.Now())
	if err != nil || startConfirmation.IdTagInfo.Status != types.AuthorizationStatusAccepted || startConfirmation.TransactionId <= 0 {
		t.Fatalf("StartTransaction confirmation=%#v err=%v", startConfirmation, err)
	}
	if _, err := c.cp.StatusNotification(1, core.NoError, core.ChargePointStatusCharging); err != nil {
		t.Fatal(err)
	}
}

func (c *contractChargePoint) meter(t *testing.T, transactionID, meterWh int64) {
	t.Helper()
	id := int(transactionID)
	_, err := c.cp.MeterValues(1, []types.MeterValue{{Timestamp: types.Now(), SampledValue: []types.SampledValue{{Value: fmt.Sprint(meterWh), Measurand: types.MeasurandEnergyActiveImportRegister, Unit: types.UnitOfMeasureWh}}}}, func(request *core.MeterValuesRequest) {
		request.TransactionId = &id
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (c *contractChargePoint) stopTransaction(t *testing.T, transactionID, meterWh int64, reason core.Reason) {
	t.Helper()
	confirmation, err := c.cp.StopTransaction(int(meterWh), types.Now(), int(transactionID), func(request *core.StopTransactionRequest) { request.Reason = reason })
	if err != nil || (confirmation != nil && confirmation.IdTagInfo != nil && confirmation.IdTagInfo.Status != types.AuthorizationStatusAccepted) {
		t.Fatalf("StopTransaction confirmation=%#v err=%v", confirmation, err)
	}
}

func (c *contractChargePoint) OnChangeAvailability(*core.ChangeAvailabilityRequest) (*core.ChangeAvailabilityConfirmation, error) {
	return core.NewChangeAvailabilityConfirmation(core.AvailabilityStatusAccepted), nil
}
func (c *contractChargePoint) OnChangeConfiguration(*core.ChangeConfigurationRequest) (*core.ChangeConfigurationConfirmation, error) {
	return core.NewChangeConfigurationConfirmation(core.ConfigurationStatusAccepted), nil
}
func (c *contractChargePoint) OnClearCache(*core.ClearCacheRequest) (*core.ClearCacheConfirmation, error) {
	return core.NewClearCacheConfirmation(core.ClearCacheStatusAccepted), nil
}
func (c *contractChargePoint) OnDataTransfer(*core.DataTransferRequest) (*core.DataTransferConfirmation, error) {
	return core.NewDataTransferConfirmation(core.DataTransferStatusAccepted), nil
}
func (c *contractChargePoint) OnGetConfiguration(*core.GetConfigurationRequest) (*core.GetConfigurationConfirmation, error) {
	return core.NewGetConfigurationConfirmation([]core.ConfigurationKey{}), nil
}
func (c *contractChargePoint) OnRemoteStartTransaction(*core.RemoteStartTransactionRequest) (*core.RemoteStartTransactionConfirmation, error) {
	return core.NewRemoteStartTransactionConfirmation(types.RemoteStartStopStatusAccepted), nil
}
func (c *contractChargePoint) OnRemoteStopTransaction(*core.RemoteStopTransactionRequest) (*core.RemoteStopTransactionConfirmation, error) {
	return core.NewRemoteStopTransactionConfirmation(types.RemoteStartStopStatusAccepted), nil
}
func (c *contractChargePoint) OnReset(*core.ResetRequest) (*core.ResetConfirmation, error) {
	return core.NewResetConfirmation(core.ResetStatusAccepted), nil
}
func (c *contractChargePoint) OnUnlockConnector(*core.UnlockConnectorRequest) (*core.UnlockConnectorConfirmation, error) {
	return core.NewUnlockConnectorConfirmation(core.UnlockStatusUnlocked), nil
}

func waitForTransaction(t *testing.T, s *store.PostgresStore, intent string) *store.V1Transaction {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		transaction, err := s.GetV1TransactionByStartIntent(context.Background(), intent)
		if err == nil {
			return transaction
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("transaction did not materialize")
	return nil
}

func waitForMeter(t *testing.T, s *store.PostgresStore, id string, sequence int64) *store.V1Transaction {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		transaction, err := s.GetV1Transaction(context.Background(), id)
		if err == nil && transaction.MeterSequence >= sequence {
			return transaction
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("meter values did not persist")
	return nil
}

func waitForWorkflow(t *testing.T, s *store.PostgresStore, id, state string) *store.V1StopWorkflow {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		workflow, err := s.GetV1StopWorkflow(context.Background(), id)
		if err == nil && workflow.State == state {
			return workflow
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("stop workflow did not reach %s", state)
	return nil
}

func waitForCompletion(t *testing.T, s *store.PostgresStore, id string) *store.V1Transaction {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		transaction, err := s.GetV1Transaction(context.Background(), id)
		if err == nil && transaction.CompletedAt != nil {
			return transaction
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("transaction did not complete")
	return nil
}

type factReceiver struct {
	*httptest.Server
	token string
	mu    sync.Mutex
	types map[string]int
}

func newFactReceiver(t *testing.T, token string) *factReceiver {
	t.Helper()
	receiver := &factReceiver{token: token, types: map[string]int{}}
	receiver.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/hal-facts" || request.Header.Get("Authorization") != "Bearer "+receiver.token || request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Idempotency-Key") == "" || request.Header.Get("X-Correlation-ID") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var envelope struct {
			FactID        string          `json:"fact_id"`
			FactType      string          `json:"fact_type"`
			SchemaVersion int             `json:"schema_version"`
			Producer      string          `json:"producer"`
			Digest        string          `json:"immutable_content_sha256"`
			Payload       json.RawMessage `json:"payload"`
		}
		if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil || envelope.FactID == "" || request.Header.Get("Idempotency-Key") != envelope.FactID || envelope.FactType == "" || envelope.SchemaVersion != 1 || envelope.Producer != "ocpp-hal-go-new" || len(envelope.Digest) != 64 || len(envelope.Payload) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		receiver.mu.Lock()
		receiver.types[envelope.FactType]++
		receiver.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	return receiver
}

func (r *factReceiver) requireTypes(t *testing.T, types ...string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, factType := range types {
		if r.types[factType] == 0 {
			t.Fatalf("receiver did not observe %s; received=%v", factType, r.types)
		}
	}
}
