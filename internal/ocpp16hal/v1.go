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
	if err := h.DispatchPendingV1Stops(ctx); err != nil {
		return err
	}
	return h.EnforceV1Deadlines(ctx)
}

// DispatchPendingV1Stops drains only PERSISTED workflows. It never replays a
// workflow that might already have crossed the OCPP network boundary.
func (h *HAL) DispatchPendingV1Stops(ctx context.Context) error {
	if h.v1Store == nil {
		return nil
	}
	workflows, err := h.v1Store.ListV1DispatchableStops(ctx, 100)
	if err != nil {
		return err
	}
	for _, workflow := range workflows {
		if _, err := h.DispatchV1Stop(ctx, workflow.HALTransactionID); err != nil && !errors.Is(err, store.ErrV1DeliveryNotReady) {
			h.logger.Warn("failed to dispatch persisted v1 stop", "hal_transaction_id", workflow.HALTransactionID, "error", err)
		}
	}
	return nil
}

func (h *HAL) EnforceV1Deadlines(ctx context.Context) error {
	if h.v1Store == nil {
		return nil
	}
	h.retryRuntimeProjections(ctx)
	transactions, err := h.v1Store.ListV1OverdueTransactions(ctx, time.Now().UTC(), 100)
	if err != nil {
		return err
	}
	for _, transaction := range transactions {
		initiator, reason := v1DeadlineStopCause(transaction.DurationLimitSource, transaction.LimitType)
		if _, _, err := h.v1Store.EnsureV1StopWorkflow(ctx, transaction.HALTransactionID, initiator, reason); err != nil {
			return err
		}
		if _, err := h.DispatchV1Stop(ctx, transaction.HALTransactionID); err != nil && !errors.Is(err, store.ErrV1DeliveryNotReady) {
			h.logger.Warn("failed to dispatch overdue v1 stop", "hal_transaction_id", transaction.HALTransactionID, "error", err)
		}
	}
	return h.DispatchPendingV1Stops(ctx)
}

func v1DeadlineStopCause(source, legacyLimitType string) (string, string) {
	if source == "CUSTOMER_MONEY" || (source == "" && legacyLimitType == "MONEY") {
		return "MONEY_LIMIT", "money_limit_reached"
	}
	if source == "WALLET" || (source == "" && legacyLimitType == "AUTO") {
		return "WALLET_LIMIT", "wallet_limit_reached"
	}
	return "TIME_LIMIT", "time_limit_reached"
}

type meterPersistenceResult uint8

const (
	meterIgnored meterPersistenceResult = iota
	meterPersisted
	meterPersistenceFailed
)

func (h *HAL) HandleV1MeterValues(chargePointID string, request *core.MeterValuesRequest) meterPersistenceResult {
	chargePointID = h.canonicalIdentity(chargePointID)
	if h.v1Store == nil || request.TransactionId == nil || *request.TransactionId <= 0 {
		return meterIgnored
	}
	telemetry := extractV1MeterTelemetry(request)
	if telemetry.Empty() {
		return meterIgnored
	}
	transaction, accepted, err := h.v1Store.UpdateV1TelemetryForOCPP(context.Background(), chargePointID, int64(*request.TransactionId), telemetry)
	if errors.Is(err, store.ErrV1TransactionNotFound) {
		return meterIgnored
	}
	if errors.Is(err, store.ErrV1InvalidEvidence) {
		return meterIgnored
	}
	if err != nil {
		h.logger.Error("failed to durably persist v1 meter", "charge_point_id", chargePointID, "error", err)
		return meterPersistenceFailed
	}
	if accepted.EnergyAccepted {
		h.registry.ApplyMeterValue(chargePointID, request.ConnectorId, ptrInt64(transaction.OCPPTransactionID), float64(*telemetry.EnergyWh))
		workflow, workflowErr := h.v1Store.GetV1StopWorkflow(context.Background(), transaction.HALTransactionID)
		if workflowErr == nil && workflow.State == "PERSISTED" {
			if _, err := h.DispatchV1Stop(context.Background(), transaction.HALTransactionID); err != nil {
				h.logger.Warn("failed to dispatch v1 energy stop", "hal_transaction_id", transaction.HALTransactionID, "error", err)
			}
		}
	}
	if accepted.EnergyAccepted || accepted.SoCAccepted {
		h.recordV1Trace(transaction.HALTransactionID, store.V1TraceEventInput{Source: "CHARGER", Target: "HAL", Category: "METER", Protocol: "OCPP1.6", Phase: "CHARGING", Summary: "Accepted transaction meter observation", OccurredAt: time.Now().UTC(), Data: traceMeterData(telemetry)})
	}
	if accepted.EnergyAccepted || accepted.SoCAccepted {
		return meterPersisted
	}
	return meterIgnored
}

func (h *HAL) HandleV1StopTransaction(chargePointID string, request *core.StopTransactionRequest) bool {
	chargePointID = h.canonicalIdentity(chargePointID)
	if h.v1Store == nil || request.TransactionId <= 0 {
		return false
	}
	transaction, err := h.v1Store.GetV1TransactionByOCPP(context.Background(), chargePointID, int64(request.TransactionId))
	if errors.Is(err, store.ErrV1TransactionNotFound) {
		return false
	}
	if err != nil {
		h.logger.Error("failed to locate v1 transaction for stop", "charge_point_id", chargePointID, "error", err)
		return false
	}
	observedAt := time.Now().UTC()
	h.recordV1Trace(transaction.HALTransactionID, store.V1TraceEventInput{Source: "CHARGER", Target: "HAL", Category: "OCPP_CALL", Protocol: "OCPP1.6", Phase: "STOPPING", Summary: "StopTransaction received", OccurredAt: observedAt, Data: map[string]any{"action": "StopTransaction", "transaction_id": request.TransactionId, "meter_wh": request.MeterStop, "reason": string(request.Reason)}})
	completedAt := observedAt
	if request.Timestamp != nil && !request.Timestamp.IsZero() {
		completedAt = request.Timestamp.UTC()
	}
	completed, err := h.v1Store.CompleteV1Transaction(context.Background(), transaction.HALTransactionID, int64(request.MeterStop), string(request.Reason), completedAt, observedAt)
	if err != nil {
		h.recordV1Trace(transaction.HALTransactionID, store.V1TraceEventInput{Source: "HAL", Target: "HAL", Category: "PERSISTENCE", Protocol: "POSTGRES", Phase: "STOPPING", Summary: "StopTransaction persistence failed", OccurredAt: observedAt, Data: map[string]any{"error_class": traceErrorClass(err)}})
		h.logger.Error("failed to complete v1 transaction", "hal_transaction_id", transaction.HALTransactionID, "error", err)
		return false
	}
	if completed.MeterStopWh == nil {
		h.logger.Error("completed v1 transaction has no effective stop meter", "hal_transaction_id", completed.HALTransactionID)
		return false
	}
	if completed.RawMeterStopWh != nil && completed.MeterStopAdjustmentWh != nil && *completed.RawMeterStopWh != *completed.MeterStopWh {
		h.logger.Info("normalized v1 stop meter quantization evidence", "hal_transaction_id", completed.HALTransactionID, "charge_point_id", chargePointID, "raw_meter_stop_wh", *completed.RawMeterStopWh, "effective_meter_stop_wh", *completed.MeterStopWh, "meter_stop_adjustment_wh", *completed.MeterStopAdjustmentWh, "meter_stop_evidence", completed.MeterStopEvidence)
	}
	h.registry.ApplyStopTransaction(chargePointID, completed.OCPPConnectorNumber, float64(*completed.MeterStopWh))
	h.recordV1Trace(completed.HALTransactionID, store.V1TraceEventInput{Source: "HAL", Target: "CMS", Category: "LIFECYCLE", Protocol: "OCPP1.6", Phase: "POST_STOP", Summary: "Transaction completion persisted", OccurredAt: observedAt, StateBefore: "ACTIVE", StateAfter: "COMPLETED", Data: map[string]any{"transaction_id": completed.OCPPTransactionID, "meter_wh": *completed.MeterStopWh, "reason": completed.OCPPStopReason}})
	return true
}

func (h *HAL) recordV1Trace(halTransactionID string, input store.V1TraceEventInput) {
	traces, ok := h.v1Store.(store.V1TraceStore)
	if !ok {
		return
	}
	trace, err := traces.FindV1TraceByTransaction(context.Background(), halTransactionID)
	if err != nil {
		return
	}
	if err := traces.AppendV1TraceEvent(context.Background(), trace.TraceID, input); err != nil {
		h.logger.Warn("failed to persist diagnostic v1 trace event", "trace_id", trace.TraceID, "error", err)
	}
}
func traceMeterData(telemetry store.V1MeterTelemetry) map[string]any {
	data := map[string]any{}
	if telemetry.EnergyWh != nil {
		data["meter_wh"] = *telemetry.EnergyWh
	}
	return data
}
func traceErrorClass(err error) string {
	if err == nil {
		return ""
	}
	return "persistence_error"
}

func ptrInt64(value int64) *int64 { return &value }

func extractV1MeterTelemetry(request *core.MeterValuesRequest) store.V1MeterTelemetry {
	telemetry := store.V1MeterTelemetry{}
	receivedAt := time.Now().UTC()
	for _, meterValue := range request.MeterValue {
		observedAt := receivedAt
		if meterValue.Timestamp != nil && !meterValue.Timestamp.IsZero() {
			observedAt = meterValue.Timestamp.UTC()
		}
		for _, sample := range meterValue.SampledValue {
			measurand := strings.TrimSpace(string(sample.Measurand))
			if strings.EqualFold(measurand, "Energy.Active.Import.Register") || strings.EqualFold(measurand, "Energy.Active.Import") {
				if value, ok := parseV1EnergyWh(sample.Value, string(sample.Unit)); ok && (telemetry.EnergyObservedAt == nil || !observedAt.Before(*telemetry.EnergyObservedAt)) {
					telemetry.EnergyWh, telemetry.EnergyObservedAt = &value, timePtr(observedAt)
				}
				continue
			}
			if strings.EqualFold(measurand, "SoC") && validV1SoCUnit(string(sample.Unit)) {
				if value, ok := store.ParseV1SoCPercent(sample.Value); ok && (telemetry.SoCObservedAt == nil || !observedAt.Before(*telemetry.SoCObservedAt)) {
					telemetry.SoCPercent, telemetry.SoCObservedAt = &value, timePtr(observedAt)
				}
			}
		}
	}
	return telemetry
}

func parseV1EnergyWh(raw, rawUnit string) (int64, bool) {
	rational, ok := new(big.Rat).SetString(strings.TrimSpace(raw))
	if !ok {
		return 0, false
	}
	unit := strings.ToLower(strings.TrimSpace(rawUnit))
	if unit == "kwh" || unit == "kilowatthour" {
		rational.Mul(rational, big.NewRat(1000, 1))
	}
	if !rational.IsInt() || !rational.Num().IsInt64() {
		return 0, false
	}
	return rational.Num().Int64(), true
}

// OCPP 1.6 allows a unit to be omitted; for the standard SoC measurand that
// means its defined percentage default. Arbitrary units are never reinterpreted.
func validV1SoCUnit(raw string) bool {
	unit := strings.ToLower(strings.TrimSpace(raw))
	return unit == "" || unit == "percent" || unit == "%"
}

func timePtr(value time.Time) *time.Time { return &value }
