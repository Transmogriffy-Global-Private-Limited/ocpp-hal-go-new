package ocpp16hal

import (
	"context"
	"net/http"
	"strings"
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
	identity, serial, ok := parseChargerIdentityPath(r, chargePointID)
	if !ok {
		h.logger.Warn("rejected malformed charge point identity path", "wire_charge_point_id", chargePointID)
		return false
	}
	if err := h.v1Store.ValidateV1ChargerAdmission(ctx, identity, serial); err != nil {
		h.logger.Warn("rejected unmapped, disabled, or conflicting charge point", "charge_point_id", identity, "serial_present", serial != "", "error", err)
		return false
	}
	if !h.rememberWireIdentity(chargePointID, identity) {
		h.logger.Warn("rejected ambiguous charge point wire identity", "wire_charge_point_id", chargePointID, "charge_point_id", identity)
		return false
	}
	return true
}

// parseChargerIdentityPath accepts exactly /{identity} or /{identity}/{serial}.
// ocpp-go uses the final path segment as its connection ID, so the returned
// canonical identity is retained separately from that wire implementation detail.
func parseChargerIdentityPath(request *http.Request, wireIdentity string) (identity, serial string, ok bool) {
	if request == nil {
		return "", "", false
	}
	path := strings.TrimPrefix(request.URL.EscapedPath(), "/")
	if path == "" || strings.Contains(path, "//") {
		return "", "", false
	}
	parts := strings.Split(path, "/")
	if len(parts) != 1 && len(parts) != 2 {
		return "", "", false
	}
	for _, part := range parts {
		if part == "" || strings.ContainsAny(part, "?#%") {
			return "", "", false
		}
	}
	if parts[len(parts)-1] != wireIdentity {
		return "", "", false
	}
	identity = parts[0]
	if len(parts) == 2 {
		serial = parts[1]
	}
	return identity, serial, true
}

func (h *HAL) rememberWireIdentity(wireIdentity, canonicalIdentity string) bool {
	h.identityMu.Lock()
	defer h.identityMu.Unlock()
	if previous, exists := h.wiredIdentity[wireIdentity]; exists && previous != canonicalIdentity {
		return false
	}
	h.wiredIdentity[wireIdentity] = canonicalIdentity
	return true
}

func (h *HAL) canonicalIdentity(wireIdentity string) string {
	h.identityMu.RLock()
	defer h.identityMu.RUnlock()
	if canonicalIdentity, exists := h.wiredIdentity[wireIdentity]; exists {
		return canonicalIdentity
	}
	return wireIdentity
}

func (h *HAL) wireIdentityFor(canonicalIdentity string) string {
	h.identityMu.RLock()
	defer h.identityMu.RUnlock()
	for wireIdentity, identity := range h.wiredIdentity {
		if identity == canonicalIdentity {
			return wireIdentity
		}
	}
	return canonicalIdentity
}

func (h *HAL) forgetWireIdentity(wireIdentity string) {
	h.identityMu.Lock()
	delete(h.wiredIdentity, wireIdentity)
	h.identityMu.Unlock()
}
