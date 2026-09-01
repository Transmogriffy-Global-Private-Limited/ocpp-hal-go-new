package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

// v1ChargingTrace is private to the CMS bearer boundary. It never provides a
// browser-facing HAL surface and deliberately returns diagnostic evidence only.
func (s *Server) v1ChargingTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	traces, ok := s.v1Store.(store.V1TraceStore)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "trace unavailable"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/charging-traces/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || !validUUID(parts[0]) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "trace_id UUID required"})
		return
	}
	trace, err := traces.GetV1Trace(r.Context(), parts[0])
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	if len(parts) == 1 {
		writeJSON(w, http.StatusOK, map[string]any{"trace": trace})
		return
	}
	if len(parts) != 2 || parts[1] != "events" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 100
	}
	var before time.Time
	beforeID := r.URL.Query().Get("before_event_id")
	if raw := r.URL.Query().Get("before_occurred_at"); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, raw)
		if parseErr != nil || !validUUID(beforeID) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "valid cursor required"})
			return
		}
		before = parsed
	}
	events, err := traces.ListV1TraceEvents(r.Context(), parts[0], before, beforeID, limit)
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func sanitizedTraceData(input map[string]any) map[string]any {
	// This allowlist is intentionally tiny. Credentials, id tags, headers and
	// customer/contact data never enter diagnostic storage.
	output := map[string]any{}
	for _, key := range []string{"action", "result", "status", "transaction_id", "connector_id", "meter_wh", "reason", "error_class"} {
		if value, ok := input[key]; ok {
			output[key] = value
		}
	}
	return output
}
