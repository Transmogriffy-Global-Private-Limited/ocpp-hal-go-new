package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
	"github.com/google/uuid"
)

const v1PathPrefix = "/v1/"

func validV1LimitType(value string) bool {
	switch value {
	case "AUTO", "ENERGY", "TIME", "MONEY":
		return true
	default:
		return false
	}
}

func validV1LimitSource(value string) bool {
	switch value {
	case "NONE", "CUSTOMER_ENERGY", "CUSTOMER_TIME", "CUSTOMER_MONEY", "WALLET":
		return true
	default:
		return false
	}
}

// normalizeV1LimitSource preserves start commands from the first customer-limit
// release, which had only limit_type. New CMS commands always provide explicit
// per-threshold provenance.
func normalizeV1LimitSource(limitType, source string, threshold int64, energy bool) (string, bool) {
	if threshold == 0 {
		return "NONE", source == "" || source == "NONE"
	}
	if source == "" {
		switch limitType {
		case "AUTO":
			source = "WALLET"
		case "MONEY":
			source = "CUSTOMER_MONEY"
		case "ENERGY":
			if energy {
				source = "CUSTOMER_ENERGY"
			}
		case "TIME":
			if !energy {
				source = "CUSTOMER_TIME"
			}
		}
	}
	if !validV1LimitSource(source) || source == "NONE" {
		return "", false
	}
	if energy && source == "CUSTOMER_TIME" {
		return "", false
	}
	if !energy && source == "CUSTOMER_ENERGY" {
		return "", false
	}
	return source, true
}

type v1MappingRequest struct {
	CPOID               string `json:"cpo_id"`
	CMSChargerID        string `json:"cms_charger_id"`
	ChargerOCPPIdentity string `json:"charger_ocpp_identity"`
	ExpectedSerial      string `json:"expected_serial,omitempty"`
	Enabled             bool   `json:"enabled"`
	Connectors          []struct {
		CMSConnectorID      string `json:"cms_connector_id"`
		OCPPConnectorNumber int    `json:"ocpp_connector_number"`
	} `json:"connectors"`
}

type v1StartRequest struct {
	TraceID             string    `json:"trace_id"`
	CMSCommandID        string    `json:"cms_command_id"`
	CMSStartIntentID    string    `json:"cms_start_intent_id"`
	CPOID               string    `json:"cpo_id"`
	CustomerID          string    `json:"customer_id"`
	CMSChargerID        string    `json:"cms_charger_id"`
	CMSConnectorID      string    `json:"cms_connector_id"`
	ChargerOCPPIdentity string    `json:"charger_ocpp_identity"`
	OCPPConnectorNumber int       `json:"ocpp_connector_number"`
	IDTag               string    `json:"id_tag"`
	CredentialExpiresAt time.Time `json:"credential_expires_at"`
	CommandExpiresAt    time.Time `json:"command_expires_at"`
	LimitType           string    `json:"limit_type"`
	EnergyLimitWh       int64     `json:"energy_limit_wh"`
	EnergyLimitSource   string    `json:"energy_limit_source,omitempty"`
	MaxDurationSeconds  int64     `json:"max_duration_seconds"`
	DurationLimitSource string    `json:"duration_limit_source,omitempty"`
}

type v1StopRequest struct {
	TraceID                string    `json:"trace_id"`
	CMSCommandID           string    `json:"cms_command_id"`
	CMSChargingSessionID   string    `json:"cms_charging_session_id"`
	CPOID                  string    `json:"cpo_id"`
	CustomerID             string    `json:"customer_id"`
	CMSChargerID           string    `json:"cms_charger_id"`
	CMSConnectorID         string    `json:"cms_connector_id"`
	ChargerOCPPIdentity    string    `json:"charger_ocpp_identity"`
	OCPPConnectorNumber    int       `json:"ocpp_connector_number"`
	HALTransactionID       string    `json:"hal_transaction_id"`
	OCPPTransactionID      int64     `json:"ocpp_transaction_id"`
	RequestedStopInitiator string    `json:"requested_stop_initiator"`
	RequestedStopReason    string    `json:"requested_stop_reason"`
	CommandExpiresAt       time.Time `json:"command_expires_at"`
}

func (s *Server) registerV1Routes(mux *http.ServeMux) {
	guard := s.requireV1Service
	mux.Handle("/v1/mappings/chargers/", guard(http.HandlerFunc(s.v1Mapping)))
	mux.Handle("/v1/remote-commands/start", guard(http.HandlerFunc(s.v1Start)))
	mux.Handle("/v1/remote-commands/stop", guard(http.HandlerFunc(s.v1Stop)))
	mux.Handle("/v1/remote-commands", guard(http.HandlerFunc(s.v1Command)))
	mux.Handle("/v1/transactions", guard(http.HandlerFunc(s.v1Transactions)))
	mux.Handle("/v1/transactions/", guard(http.HandlerFunc(s.v1Transaction)))
	mux.Handle("/v1/facts/", guard(http.HandlerFunc(s.v1FactRequeue)))
	mux.Handle("/v1/runtime/chargers/", guard(http.HandlerFunc(s.v1ChargerRuntime)))
	mux.Handle("/v1/runtime/connectors/", guard(http.HandlerFunc(s.v1ConnectorRuntime)))
}

// sanitizedTraceData is the HAL persistence-side defence for diagnostic
// evidence. The CMS receiver independently enforces the same small contract.
func sanitizedTraceData(input map[string]any) map[string]any {
	output := map[string]any{}
	for _, key := range []string{"action", "result", "status", "transaction_id", "connector_id", "meter_wh", "reason", "error_class"} {
		if value, ok := input[key]; ok {
			output[key] = value
		}
	}
	return output
}

func (s *Server) requireV1Service(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(header, prefix) || s.cfg.V1CMSBearerToken == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "v1 service authentication required"})
			return
		}
		provided := strings.TrimPrefix(header, prefix)
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.V1CMSBearerToken)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "v1 service authentication required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) v1Mapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	correlation, idempotency, ok := v1MutationHeaders(w, r)
	if !ok {
		return
	}
	var req v1MappingRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	pathID := strings.TrimPrefix(r.URL.Path, "/v1/mappings/chargers/")
	if pathID == "" || pathID != req.CMSChargerID || !validUUID(req.CPOID) || !validUUID(req.CMSChargerID) || strings.TrimSpace(req.ChargerOCPPIdentity) == "" || len(req.ChargerOCPPIdentity) > 255 || !validIdentityEvidence(req.ExpectedSerial) || len(req.Connectors) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid immutable charger mapping"})
		return
	}
	connectors := make([]store.V1ConnectorMappingInput, 0, len(req.Connectors))
	seen := map[int]bool{}
	for _, c := range req.Connectors {
		if !validUUID(c.CMSConnectorID) || c.OCPPConnectorNumber <= 0 || seen[c.OCPPConnectorNumber] {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid connector mapping"})
			return
		}
		seen[c.OCPPConnectorNumber] = true
		connectors = append(connectors, store.V1ConnectorMappingInput{CMSConnectorID: c.CMSConnectorID, OCPPConnectorNumber: c.OCPPConnectorNumber})
	}
	digest := digestJSON(req, idempotency)
	mapping, existed, err := s.v1Store.SyncV1Mapping(r.Context(), store.V1MappingInput{CPOID: req.CPOID, CMSChargerID: req.CMSChargerID, ChargerOCPPIdentity: req.ChargerOCPPIdentity, ExpectedSerial: strings.TrimSpace(req.ExpectedSerial), Enabled: req.Enabled, Connectors: connectors, CorrelationID: correlation, RequestDigest: digest})
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	status := http.StatusCreated
	if existed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"mapping": v1MappingView(mapping), "idempotency_key": idempotency, "correlation_id": correlation})
}

func validIdentityEvidence(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) > 100 {
		return false
	}
	for _, runeValue := range value {
		if runeValue < 0x21 || runeValue > 0x7e || runeValue == '/' {
			return false
		}
	}
	return true
}

func (s *Server) v1Start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	correlation, idempotency, ok := v1MutationHeaders(w, r)
	if !ok {
		return
	}
	var req v1StartRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	// CMS versions released before customer-selected limits did not send this
	// field. Their former two-limit shape is semantically AUTO.
	if req.LimitType == "" {
		req.LimitType = "AUTO"
	}
	var sourcesValid bool
	req.EnergyLimitSource, sourcesValid = normalizeV1LimitSource(req.LimitType, req.EnergyLimitSource, req.EnergyLimitWh, true)
	if sourcesValid {
		req.DurationLimitSource, sourcesValid = normalizeV1LimitSource(req.LimitType, req.DurationLimitSource, req.MaxDurationSeconds, false)
	}
	if r.Header.Get("Idempotency-Key") != req.CMSCommandID || (req.TraceID != "" && !validUUID(req.TraceID)) || !validUUID(req.CMSCommandID) || !validUUID(req.CMSStartIntentID) || !validUUID(req.CPOID) || !validUUID(req.CustomerID) || !validUUID(req.CMSChargerID) || !validUUID(req.CMSConnectorID) || req.OCPPConnectorNumber <= 0 || !strings.HasPrefix(req.IDTag, "appv1_") || len(req.IDTag) > 20 || !validV1LimitType(req.LimitType) || req.EnergyLimitWh < 0 || req.MaxDurationSeconds < 0 || !sourcesValid || !req.CredentialExpiresAt.After(time.Now().UTC()) || !req.CommandExpiresAt.After(req.CredentialExpiresAt) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid start command"})
		return
	}
	if err := s.v1Store.ValidateV1Mapping(r.Context(), req.CPOID, req.CMSChargerID, req.CMSConnectorID, req.ChargerOCPPIdentity, req.OCPPConnectorNumber); err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	var energy, duration *int64
	if req.EnergyLimitWh > 0 {
		energy = &req.EnergyLimitWh
	}
	if req.MaxDurationSeconds > 0 {
		duration = &req.MaxDurationSeconds
	}
	command, existed, err := s.v1Store.CreateV1StartCommand(r.Context(), store.V1StartCommandInput{CMSCommandID: req.CMSCommandID, RequestDigest: digestJSON(req, idempotency), CPOID: req.CPOID, CustomerID: req.CustomerID, CorrelationID: correlation, CMSStartIntentID: req.CMSStartIntentID, CMSChargerID: req.CMSChargerID, CMSConnectorID: req.CMSConnectorID, ChargerOCPPIdentity: req.ChargerOCPPIdentity, OCPPConnectorNumber: req.OCPPConnectorNumber, IDTag: req.IDTag, CredentialExpiresAt: req.CredentialExpiresAt, CommandExpiresAt: req.CommandExpiresAt, LimitType: req.LimitType, EnergyLimitWh: energy, EnergyLimitSource: req.EnergyLimitSource, MaxDurationSeconds: duration, DurationLimitSource: req.DurationLimitSource})
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	if traces, ok := s.v1Store.(store.V1TraceStore); ok && req.TraceID != "" {
		if _, err := traces.EnsureV1Trace(r.Context(), store.V1Trace{TraceID: req.TraceID, CPOID: req.CPOID, CMSStartIntentID: req.CMSStartIntentID, CMSCommandID: req.CMSCommandID, ChargerOCPPIdentity: req.ChargerOCPPIdentity, OCPPConnectorNumber: req.OCPPConnectorNumber}); err != nil {
			s.logger.Warn("failed to persist diagnostic start trace root", "trace_id", req.TraceID, "error", err)
		} else {
			s.appendV1Trace(r.Context(), traces, req.TraceID, store.V1TraceEventInput{Source: "CMS", Target: "HAL", Category: "COMMAND", Protocol: "HTTP", Phase: "STARTING", Summary: "CMS start command accepted by HAL", OccurredAt: time.Now().UTC(), CorrelationID: correlation})
		}
	}
	command, claimed, err := s.v1Store.ClaimV1StartDelivery(r.Context(), req.CMSCommandID)
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	if claimed {
		command, err = s.v1Store.BeginV1CommandDelivery(r.Context(), req.CMSCommandID)
		if err != nil {
			s.writeV1StoreError(w, err)
			return
		}
		status, callErr := s.hal.RemoteStartTransaction(r.Context(), req.ChargerOCPPIdentity, req.IDTag, req.OCPPConnectorNumber)
		if traces, ok := s.v1Store.(store.V1TraceStore); ok && req.TraceID != "" {
			outcome := "RemoteStartTransaction outcome unavailable"
			if callErr == nil {
				outcome = "RemoteStartTransaction response received"
			}
			s.appendV1Trace(r.Context(), traces, req.TraceID, store.V1TraceEventInput{Source: "HAL", Target: "CHARGER", Category: "OCPP_CALL", Protocol: "OCPP1.6", Phase: "STARTING", Summary: outcome, OccurredAt: time.Now().UTC(), CorrelationID: correlation, Data: sanitizedTraceData(map[string]any{"action": "RemoteStartTransaction", "result": status})})
		}
		if callErr != nil {
			command, err = s.v1Store.MarkV1CommandDelivery(r.Context(), req.CMSCommandID, "AMBIGUOUS", "", "remote start result unavailable")
			if err != nil {
				s.writeV1StoreError(w, err)
				return
			}
		} else {
			command, err = s.v1Store.MarkV1CommandDelivery(r.Context(), req.CMSCommandID, status, status, "")
			if err != nil {
				s.writeV1StoreError(w, err)
				return
			}
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"command": v1CommandView(command), "correlation_id": correlation, "duplicate": existed})
}

func (s *Server) v1Stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	correlation, idempotency, ok := v1MutationHeaders(w, r)
	if !ok {
		return
	}
	var req v1StopRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if r.Header.Get("Idempotency-Key") != req.CMSCommandID || (req.TraceID != "" && !validUUID(req.TraceID)) || !validUUID(req.CMSCommandID) || !validUUID(req.CMSChargingSessionID) || !validUUID(req.CPOID) || !validUUID(req.CMSChargerID) || !validUUID(req.CMSConnectorID) || !validUUID(req.HALTransactionID) || req.OCPPTransactionID <= 0 || req.OCPPConnectorNumber <= 0 || !req.CommandExpiresAt.After(time.Now().UTC()) || !validV1StopInitiator(req.RequestedStopInitiator) || strings.TrimSpace(req.RequestedStopReason) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid stop command"})
		return
	}
	command, duplicate, err := s.v1Store.CreateV1StopCommand(r.Context(), store.V1StopCommandInput{CMSCommandID: req.CMSCommandID, RequestDigest: digestJSON(req, idempotency), CPOID: req.CPOID, CustomerID: req.CustomerID, CorrelationID: correlation, CMSChargingSessionID: req.CMSChargingSessionID, CMSChargerID: req.CMSChargerID, CMSConnectorID: req.CMSConnectorID, ChargerOCPPIdentity: req.ChargerOCPPIdentity, OCPPConnectorNumber: req.OCPPConnectorNumber, HALTransactionID: req.HALTransactionID, OCPPTransactionID: req.OCPPTransactionID, RequestedStopInitiator: req.RequestedStopInitiator, RequestedStopReason: req.RequestedStopReason, CommandExpiresAt: req.CommandExpiresAt})
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	if traces, ok := s.v1Store.(store.V1TraceStore); ok && req.TraceID != "" {
		if err := traces.BindV1TraceTransaction(r.Context(), req.TraceID, &store.V1Transaction{HALTransactionID: req.HALTransactionID, OCPPTransactionID: req.OCPPTransactionID}); err != nil {
			s.logger.Warn("failed to bind diagnostic stop trace transaction", "trace_id", req.TraceID, "error", err)
		}
		s.appendV1Trace(r.Context(), traces, req.TraceID, store.V1TraceEventInput{Source: "CMS", Target: "HAL", Category: "COMMAND", Protocol: "HTTP", Phase: "STOPPING", Summary: "CMS stop command accepted by HAL", OccurredAt: time.Now().UTC(), CorrelationID: correlation})
	}
	workflow, dispatchErr := s.hal.DispatchV1Stop(r.Context(), req.HALTransactionID)
	if dispatchErr != nil {
		s.logger.Warn("v1 stop dispatch did not produce a final command result", "hal_transaction_id", req.HALTransactionID, "error", dispatchErr)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"command": v1CommandView(command), "stop_workflow": v1StopWorkflowView(workflow), "correlation_id": correlation, "duplicate": duplicate})
}

// appendV1Trace records diagnostic evidence without changing command semantics.
// Trace persistence failures must remain observable but cannot turn an otherwise
// accepted OCPP/CMS command into a different protocol result.
func (s *Server) appendV1Trace(ctx context.Context, traces store.V1TraceStore, traceID string, input store.V1TraceEventInput) {
	if err := traces.AppendV1TraceEvent(ctx, traceID, input); err != nil {
		s.logger.Warn("failed to persist diagnostic v1 trace event", "trace_id", traceID, "error", err)
	}
}

func validV1StopInitiator(value string) bool {
	switch value {
	case "CUSTOMER", "CPO":
		return true
	default:
		return false
	}
}

func (s *Server) v1Command(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("cms_command_id")
	if !validUUID(id) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "cms_command_id UUID required"})
		return
	}
	c, err := s.v1Store.GetV1Command(r.Context(), id)
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"command": v1CommandView(c)})
}
func (s *Server) v1Transactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("cms_start_intent_id")
	if !validUUID(id) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "cms_start_intent_id UUID required"})
		return
	}
	t, err := s.v1Store.GetV1TransactionByStartIntent(r.Context(), id)
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	response := map[string]any{"transaction": v1TransactionView(t)}
	if workflow, err := s.v1Store.GetV1StopWorkflow(r.Context(), t.HALTransactionID); err == nil {
		response["stop_workflow"] = v1StopWorkflowView(workflow)
	}
	writeJSON(w, http.StatusOK, response)
}
func (s *Server) v1Transaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/transactions/")
	if !validUUID(id) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "hal_transaction_id UUID required"})
		return
	}
	t, err := s.v1Store.GetV1Transaction(r.Context(), id)
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	response := map[string]any{"transaction": v1TransactionView(t)}
	if workflow, err := s.v1Store.GetV1StopWorkflow(r.Context(), t.HALTransactionID); err == nil {
		response["stop_workflow"] = v1StopWorkflowView(workflow)
	}
	writeJSON(w, http.StatusOK, response)
}
func (s *Server) v1ChargerRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	identity := strings.TrimPrefix(r.URL.Path, "/v1/runtime/chargers/")
	if identity == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "charger identity required"})
		return
	}
	state, err := s.v1Store.GetV1ChargerRuntime(r.Context(), identity)
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runtime": v1ChargerRuntimeView(state), "fresh": state.ConnectionState == "ONLINE"})
}
func (s *Server) v1ConnectorRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/runtime/connectors/")
	if !validUUID(id) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "cms_connector_id UUID required"})
		return
	}
	state, err := s.v1Store.GetV1ConnectorRuntime(r.Context(), id)
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	charger, err := s.v1Store.GetV1ChargerRuntime(r.Context(), state.ChargerOCPPIdentity)
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runtime": v1ConnectorRuntimeView(*state), "fresh": charger.ConnectionState == "ONLINE"})
}

func (s *Server) v1FactRequeue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	factID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/facts/"), "/requeue")
	if !strings.HasSuffix(r.URL.Path, "/requeue") || !validUUID(factID) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "fact_id UUID required"})
		return
	}
	correlationID := strings.TrimSpace(r.Header.Get("X-Correlation-ID"))
	if !validUUID(correlationID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Correlation-ID must be a canonical UUID"})
		return
	}
	if err := s.v1Store.RequeueV1Fact(r.Context(), factID, correlationID); err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"fact_id": factID, "status": "PENDING"})
}

func v1MutationHeaders(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	idempotency := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	correlation := strings.TrimSpace(r.Header.Get("X-Correlation-ID"))
	if !validUUID(idempotency) || !validUUID(correlation) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Idempotency-Key and X-Correlation-ID must be canonical UUIDs"})
		return "", "", false
	}
	return correlation, idempotency, true
}
func digestJSON(v any, idempotency string) string {
	data, _ := json.Marshal(v)
	sum := sha256.Sum256(append(data, []byte("\n"+idempotency)...))
	return hex.EncodeToString(sum[:])
}
func validUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil && parsed.String() == value
}
func (s *Server) writeV1StoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrV1MappingNotFound), errors.Is(err, store.ErrV1CommandNotFound), errors.Is(err, store.ErrV1TransactionNotFound), errors.Is(err, store.ErrV1FactNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "v1 resource not found"})
	case errors.Is(err, store.ErrV1MappingConflict), errors.Is(err, store.ErrV1IdempotencyConflict), errors.Is(err, store.ErrV1FactNotReconciliable):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "v1 idempotency or mapping conflict"})
	case errors.Is(err, store.ErrV1CredentialRejected):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "v1 request is not permitted by current mapping state"})
	case errors.Is(err, store.ErrV1InvalidEvidence):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid v1 transaction evidence"})
	default:
		s.logger.Error("v1 persistence operation failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "v1 persistence operation failed"})
	}
}
