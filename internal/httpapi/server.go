package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/ocpp16hal"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

// Server exposes only the authenticated v1 CMS-to-HAL boundary. Customer and
// CPO-facing projections belong to the CMS, not to this protocol service.
type Server struct {
	cfg     config.Config
	logger  *slog.Logger
	hal     *ocpp16hal.HAL
	v1Store store.V1Store
}

func NewServer(cfg config.Config, logger *slog.Logger, hal *ocpp16hal.HAL, v1Store store.V1Store) *Server {
	return &Server{cfg: cfg, logger: logger, hal: hal, v1Store: v1Store}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	if s.v1Store != nil && s.hal != nil {
		s.registerV1Routes(mux)
	}
	if s.cfg.APIDocsEnabled {
		s.registerAPIDocs(mux)
	}
	return mux
}

func writeJSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(body)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON request"})
		return false
	}
	return true
}
