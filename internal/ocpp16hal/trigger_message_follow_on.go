package ocpp16hal

import (
	"context"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

const triggerMessageFollowOnObservationWindow = time.Minute

// recordTriggerMessageFollowOn duplicates a temporally matching charger CALL
// into every accepted TriggerMessage trace window. OCPP provides no causal
// correlation between those messages, so this is observation evidence only.
func (h *HAL) recordTriggerMessageFollowOn(chargerID, action string, connector int, observedAt time.Time) {
	if h == nil || h.v1Store == nil || !store.V1TriggerMessageAction(action) || (store.V1TriggerMessageConnectorScoped(action) && connector < 1) {
		return
	}
	expectations, ok := h.v1Store.(store.V1TriggerMessageFollowOnExpectationStore)
	if !ok {
		return
	}
	traces, ok := h.v1Store.(store.V1TraceStore)
	if !ok {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	matches, err := expectations.ListV1TriggerMessageFollowOnExpectations(context.Background(), chargerID, action, connector, store.V1TriggerMessageConnectorScoped(action), observedAt.Add(-triggerMessageFollowOnObservationWindow), observedAt)
	if err != nil {
		h.logger.Warn("failed to find TriggerMessage follow-on expectations", "charge_point_id", chargerID, "action", action, "error", err)
		return
	}
	for _, match := range matches {
		data := map[string]any{"follow_on": true, "expected_message": match.RequestedMessage, "observed_action": action, "charger_ocpp_identity": chargerID}
		if store.V1TriggerMessageConnectorScoped(action) {
			data["connector_number"] = connector
		}
		if err := traces.AppendV1TraceEvent(context.Background(), match.TraceID, store.V1TraceEventInput{Source: "CHARGER", Target: "HAL", Category: "CHARGER_OPERATION_FOLLOW_ON", Protocol: "OCPP1.6", Phase: "CHARGING", Summary: "TriggerMessage follow-on observed", OccurredAt: observedAt, Data: data}); err != nil {
			h.logger.Warn("failed to persist TriggerMessage follow-on diagnostic evidence", "trace_id", match.TraceID, "action", action, "error", err)
		}
	}
}
