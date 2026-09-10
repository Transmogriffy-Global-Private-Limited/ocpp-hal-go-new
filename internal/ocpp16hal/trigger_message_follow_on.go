package ocpp16hal

import (
	"context"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

// recordTriggerMessageFollowOn duplicates a temporally matching charger CALL
// into every accepted TriggerMessage trace window. OCPP provides no causal
// correlation between those messages, so this is observation evidence only.
func (h *HAL) recordTriggerMessageFollowOn(chargerID, action string, connector int, observedAt time.Time) {
	if h == nil || h.v1Store == nil || !store.V1TriggerMessageAction(action) || (store.V1TriggerMessageConnectorScoped(action) && connector < 1) {
		return
	}
	windows, ok := h.v1Store.(store.V1TriggerMessageFollowOnWindowStore)
	if !ok {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	_, err := windows.RecordV1TriggerMessageFollowOn(context.Background(), chargerID, action, connector, observedAt)
	if err != nil {
		h.logger.Warn("failed to persist TriggerMessage follow-on diagnostic evidence", "charge_point_id", chargerID, "action", action, "error", err)
	}
}
