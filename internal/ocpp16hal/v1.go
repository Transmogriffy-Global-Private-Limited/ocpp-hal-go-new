package ocpp16hal

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

// DispatchV1Stop is the sole HAL-side RemoteStop dispatcher. The store claim
// and recorded DELIVERY_ATTEMPTED state prevent a second worker from guessing
// that it may safely send the same OCPP command after a crash.
func (h *HAL) DispatchV1Stop(ctx context.Context, halTransactionID string) (*store.V1StopWorkflow, error) {
	if h.v1Store == nil {
		return nil, store.ErrV1TransactionNotFound
	}
	workflow, claimed, err := h.v1Store.ClaimV1StopDelivery(ctx, halTransactionID)
	if err != nil || !claimed {
		return workflow, err
	}
	if _, err = h.v1Store.BeginV1StopDelivery(ctx, halTransactionID); err != nil {
		return nil, err
	}
	transaction, err := h.v1Store.GetV1Transaction(ctx, halTransactionID)
	if err != nil {
		return nil, err
	}
	if transaction.CompletedAt != nil {
		return h.v1Store.MarkV1StopDelivery(ctx, halTransactionID, "AMBIGUOUS", "", "transaction completed before dispatch")
	}
	callCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	status, callErr := h.RemoteStopTransaction(callCtx, transaction.ChargerOCPPIdentity, int(transaction.OCPPTransactionID))
	if callErr != nil {
		return h.v1Store.MarkV1StopDelivery(ctx, halTransactionID, "AMBIGUOUS", "", "remote stop result unavailable")
	}
	return h.v1Store.MarkV1StopDelivery(ctx, halTransactionID, status, status, "")
}

// RecoverV1Lifecycle marks only delivery windows with proven pre-network state
// as replayable. A durable DELIVERY_ATTEMPTED marker is intentionally left as
// AMBIGUOUS because the wire outcome cannot be reconstructed safely.
func (h *HAL) RecoverV1Lifecycle(ctx context.Context) error {
	if h.v1Store == nil {
		return nil
	}
	if err := h.v1Store.RecoverV1CommandDelivery(ctx); err != nil {
		return err
	}
	if err := h.v1Store.RecoverV1StopDelivery(ctx); err != nil {
		return err
	}
	return h.EnforceV1Deadlines(ctx)
}

func (h *HAL) EnforceV1Deadlines(ctx context.Context) error {
	if h.v1Store == nil {
		return nil
	}
	transactions, err := h.v1Store.ListV1OverdueTransactions(ctx, time.Now().UTC(), 100)
	if err != nil {
		return err
	}
	for _, transaction := range transactions {
		if _, _, err := h.v1Store.EnsureV1StopWorkflow(ctx, transaction.HALTransactionID, "TIME_LIMIT", "time_limit_reached"); err != nil {
			return err
		}
		if _, err := h.DispatchV1Stop(ctx, transaction.HALTransactionID); err != nil && !errors.Is(err, store.ErrV1DeliveryNotReady) {
			h.logger.Warn("failed to dispatch overdue v1 stop", "hal_transaction_id", transaction.HALTransactionID, "error", err)
		}
	}
	return nil
}

func (h *HAL) HandleV1MeterValues(chargePointID string, request *core.MeterValuesRequest) bool {
	if h.v1Store == nil || request.TransactionId == nil || *request.TransactionId <= 0 {
		return false
	}
	meterWh, observedAt, ok := extractV1MeterValueWh(request)
	if !ok {
		return false
	}
	transaction, accepted, err := h.v1Store.UpdateV1MeterForOCPP(context.Background(), chargePointID, int64(*request.TransactionId), meterWh, observedAt)
	if errors.Is(err, store.ErrV1TransactionNotFound) {
		return false
	}
	if err != nil {
		h.logger.Warn("failed to persist v1 meter", "charge_point_id", chargePointID, "error", err)
		return true
	}
	if accepted {
		h.registry.ApplyMeterValue(chargePointID, request.ConnectorId, ptrInt64(transaction.OCPPTransactionID), float64(meterWh))
		workflow, workflowErr := h.v1Store.GetV1StopWorkflow(context.Background(), transaction.HALTransactionID)
		if workflowErr == nil && workflow.State == "PERSISTED" {
			if _, err := h.DispatchV1Stop(context.Background(), transaction.HALTransactionID); err != nil {
				h.logger.Warn("failed to dispatch v1 energy stop", "hal_transaction_id", transaction.HALTransactionID, "error", err)
			}
		}
	}
	return true
}

func (h *HAL) HandleV1StopTransaction(chargePointID string, request *core.StopTransactionRequest) bool {
	if h.v1Store == nil || request.TransactionId <= 0 {
		return false
	}
	transaction, err := h.v1Store.GetV1TransactionByOCPP(context.Background(), chargePointID, int64(request.TransactionId))
	if errors.Is(err, store.ErrV1TransactionNotFound) {
		return false
	}
	if err != nil {
		h.logger.Warn("failed to locate v1 transaction for stop", "charge_point_id", chargePointID, "error", err)
		return true
	}
	completedAt := time.Now().UTC()
	if request.Timestamp != nil && !request.Timestamp.IsZero() {
		completedAt = request.Timestamp.UTC()
	}
	completed, err := h.v1Store.CompleteV1Transaction(context.Background(), transaction.HALTransactionID, int64(request.MeterStop), string(request.Reason), completedAt)
	if err != nil {
		h.logger.Error("failed to complete v1 transaction", "hal_transaction_id", transaction.HALTransactionID, "error", err)
		return true
	}
	h.registry.ApplyStopTransaction(chargePointID, completed.OCPPConnectorNumber, float64(request.MeterStop))
	return true
}

func ptrInt64(value int64) *int64 { return &value }

func extractV1MeterValueWh(request *core.MeterValuesRequest) (int64, time.Time, bool) {
	for i := len(request.MeterValue) - 1; i >= 0; i-- {
		meterValue := request.MeterValue[i]
		for _, sample := range meterValue.SampledValue {
			measurand := strings.TrimSpace(string(sample.Measurand))
			if !strings.EqualFold(measurand, "Energy.Active.Import.Register") && !strings.EqualFold(measurand, "Energy.Active.Import") {
				continue
			}
			rational, ok := new(big.Rat).SetString(strings.TrimSpace(sample.Value))
			if !ok {
				continue
			}
			unit := strings.ToLower(strings.TrimSpace(string(sample.Unit)))
			if unit == "kwh" || unit == "kilowatthour" {
				rational.Mul(rational, big.NewRat(1000, 1))
			}
			if !rational.IsInt() || !rational.Num().IsInt64() {
				continue
			}
			observedAt := time.Now().UTC()
			if meterValue.Timestamp != nil && !meterValue.Timestamp.IsZero() {
				observedAt = meterValue.Timestamp.UTC()
			}
			return rational.Num().Int64(), observedAt, true
		}
	}
	return 0, time.Time{}, false
}
