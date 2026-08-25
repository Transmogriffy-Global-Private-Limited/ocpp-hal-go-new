package ocpp16hal

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

type failingV1CompletionStore struct{ store.V1Store }

type bootEvidenceStore struct {
	store.V1Store
	identity string
	evidence store.V1BootEvidence
	calls    int
}

func (s *bootEvidenceStore) RecordV1BootEvidence(_ context.Context, identity string, evidence store.V1BootEvidence) error {
	s.identity, s.evidence, s.calls = identity, evidence, s.calls+1
	return nil
}

func TestBootMetadataIsRecordedAsEvidenceWithoutChangingCanonicalIdentity(t *testing.T) {
	s := &bootEvidenceStore{}
	h := New(state.NewRegistry(), s, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if !h.rememberWireIdentity("SN-1", "CP-1") {
		t.Fatal("could not establish test identity alias")
	}
	confirmation, err := h.OnBootNotification("SN-1", &core.BootNotificationRequest{ChargePointVendor: "Vendor", ChargePointModel: "Model", ChargePointSerialNumber: "Boot-SN"})
	if err != nil || confirmation == nil || s.calls != 1 {
		t.Fatalf("confirmation=%#v err=%v calls=%d", confirmation, err, s.calls)
	}
	if s.identity != "CP-1" || s.evidence.PathSerial != "SN-1" || s.evidence.ChargePointSerialNumber != "Boot-SN" {
		t.Fatalf("identity=%q evidence=%#v", s.identity, s.evidence)
	}
}

type failingV1ObservationStore struct {
	store.V1Store
	statusErr error
	meterErr  error
}

func (s failingV1ObservationStore) RecordV1ConnectorStatus(context.Context, store.V1ConnectorRuntime) error {
	return s.statusErr
}

func (s failingV1ObservationStore) UpdateV1TelemetryForOCPP(context.Context, string, int64, store.V1MeterTelemetry) (*store.V1Transaction, store.V1TelemetryUpdateResult, error) {
	return nil, store.V1TelemetryUpdateResult{}, s.meterErr
}

func (failingV1CompletionStore) GetV1TransactionByOCPP(context.Context, string, int64) (*store.V1Transaction, error) {
	return &store.V1Transaction{HALTransactionID: store.MustNewUUIDString(), ChargerOCPPIdentity: "CP-persistence", OCPPConnectorNumber: 1, OCPPTransactionID: 9, ActualStartedAt: time.Now().UTC(), ObservedStartedAt: time.Now().UTC()}, nil
}

func (failingV1CompletionStore) CompleteV1Transaction(context.Context, string, int64, string, time.Time, time.Time) (*store.V1Transaction, error) {
	return nil, errors.New("database unavailable")
}

func TestOnStopTransactionReturnsErrorWhenPersistenceFails(t *testing.T) {
	h := New(state.NewRegistry(), failingV1CompletionStore{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	confirmation, err := h.OnStopTransaction("CP-persistence", &core.StopTransactionRequest{TransactionId: 9, MeterStop: 100})
	if err == nil || confirmation != nil {
		t.Fatalf("confirmation=%#v err=%v", confirmation, err)
	}
}

func TestStatusNotificationDoesNotAcknowledgePersistenceFailure(t *testing.T) {
	registry := state.NewRegistry()
	h := New(registry, failingV1ObservationStore{statusErr: errors.New("database unavailable")}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	confirmation, err := h.OnStatusNotification("CP-persistence", &core.StatusNotificationRequest{ConnectorId: 1})
	if err == nil || confirmation != nil {
		t.Fatalf("confirmation=%#v err=%v", confirmation, err)
	}
	if _, exists := registry.Snapshot("CP-persistence"); exists {
		t.Fatal("local connector projection advanced without durable status evidence")
	}
}

func TestMeterValuesAcknowledgesUnsupportedButNotPersistenceFailure(t *testing.T) {
	transactionID := 9
	h := New(state.NewRegistry(), failingV1ObservationStore{meterErr: errors.New("database unavailable")}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	unsupported, err := h.OnMeterValues("CP-persistence", &core.MeterValuesRequest{TransactionId: &transactionID})
	if err != nil || unsupported == nil {
		t.Fatalf("unsupported confirmation=%#v err=%v", unsupported, err)
	}
	// The valid-energy construction is covered at the extraction boundary; the
	// store failure path must become a CALLERROR rather than an empty success.
	request := meterRequest(transactionID)
	confirmation, err := h.OnMeterValues("CP-persistence", request)
	if err == nil || confirmation != nil {
		t.Fatalf("confirmation=%#v err=%v", confirmation, err)
	}
}

func meterRequest(transactionID int) *core.MeterValuesRequest {
	return &core.MeterValuesRequest{
		ConnectorId:   1,
		TransactionId: &transactionID,
		MeterValue: []types.MeterValue{{
			Timestamp: types.NewDateTime(time.Now().UTC()),
			SampledValue: []types.SampledValue{{
				Value: "100", Measurand: types.MeasurandEnergyActiveImportRegister, Unit: types.UnitOfMeasureWh,
			}},
		}},
	}
}
