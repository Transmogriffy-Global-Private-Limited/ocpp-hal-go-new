package ocpp16hal

import (
	"context"
	"net/http"
)

// validateIncomingCharger admits only an enabled durable v1 mapping. The
// mapping is the new HAL's charger-admission authority; it never falls back to
// an external CMS directory or a permissive unknown-charger path.
func (h *HAL) validateIncomingCharger(chargePointID string, r *http.Request) bool {
	if h.v1Store == nil {
		return false
	}
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	if err := h.v1Store.ValidateV1ChargerAdmission(ctx, chargePointID); err != nil {
		h.logger.Warn("rejected unmapped or disabled charge point", "charge_point_id", chargePointID, "error", err)
		return false
	}
	return true
}
