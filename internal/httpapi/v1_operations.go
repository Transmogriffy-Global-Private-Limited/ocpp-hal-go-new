package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

type v1ChargerOperationRequest struct {
	CMSOperationID      string            `json:"cms_operation_id"`
	CPOID               string            `json:"cpo_id"`
	CMSChargerID        string            `json:"cms_charger_id"`
	CMSConnectorID      string            `json:"cms_connector_id,omitempty"`
	ChargerOCPPIdentity string            `json:"charger_ocpp_identity"`
	OCPPConnectorNumber int               `json:"ocpp_connector_number"`
	Kind                string            `json:"kind"`
	Parameters          map[string]string `json:"parameters"`
}

type v1ConfigurationReadRequest struct {
	CPOID               string   `json:"cpo_id"`
	CMSChargerID        string   `json:"cms_charger_id"`
	ChargerOCPPIdentity string   `json:"charger_ocpp_identity"`
	Keys                []string `json:"keys,omitempty"`
}

type v1ChargerOperationMappingValidator interface {
	ValidateV1ChargerOperationMapping(context.Context, string, string, string, string, int) error
}

var v1TriggerMessageAllowlist = map[string]bool{
	"BootNotification": true, "DiagnosticsStatusNotification": true,
	"FirmwareStatusNotification": true, "Heartbeat": true,
	"MeterValues": true, "StatusNotification": true,
}

func validV1ChargerOperation(request v1ChargerOperationRequest) bool {
	if !validUUID(request.CMSOperationID) || !validUUID(request.CPOID) || !validUUID(request.CMSChargerID) || strings.TrimSpace(request.ChargerOCPPIdentity) == "" || len(request.ChargerOCPPIdentity) > 255 || request.OCPPConnectorNumber < 0 {
		return false
	}
	if request.OCPPConnectorNumber == 0 && request.CMSConnectorID != "" {
		return false
	}
	if request.OCPPConnectorNumber > 0 && !validUUID(request.CMSConnectorID) {
		return false
	}
	switch request.Kind {
	case "RESET":
		return request.OCPPConnectorNumber == 0 && (request.Parameters["type"] == "SOFT" || request.Parameters["type"] == "HARD")
	case "UNLOCK_CONNECTOR":
		return request.OCPPConnectorNumber > 0 && len(request.Parameters) == 0
	case "CHANGE_AVAILABILITY":
		return request.Parameters["type"] == "OPERATIVE" || request.Parameters["type"] == "INOPERATIVE"
	case "CLEAR_CACHE":
		return request.OCPPConnectorNumber == 0 && len(request.Parameters) == 0
	case "CHANGE_CONFIGURATION":
		return request.OCPPConnectorNumber == 0 && validV1ConfigurationChange(request.Parameters)
	case "TRIGGER_MESSAGE":
		return v1TriggerMessageAllowlist[request.Parameters["requested_message"]] && len(request.Parameters) == 1
	default:
		return false
	}
}

func validV1ConfigurationChange(parameters map[string]string) bool {
	if len(parameters) != 2 {
		return false
	}
	key, value := strings.TrimSpace(parameters["key"]), parameters["value"]
	return validV1ConfigurationKey(key) && len(value) <= 500 && printableV1ConfigurationValue(value) && !v1HALOwnedConfigurationKey(key) && !v1SensitiveConfigurationKey(key)
}

func validV1ConfigurationKey(value string) bool {
	if len(value) < 1 || len(value) > 100 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}

func printableV1ConfigurationValue(value string) bool {
	for _, r := range value {
		if r < 0x20 || r > 0x7e {
			return false
		}
	}
	return true
}

func v1HALOwnedConfigurationKey(key string) bool {
	switch strings.ToLower(key) {
	case "heartbeatinterval", "metervalueinterval", "metervaluesampleinterval", "authorizeremotetxrequests", "localauthorizeoffline", "localpreauthorize", "authorizationcacheenabled", "allowofflinetxforunknownid", "stoptransactiononinvalidid", "chargepointauthenable", "freevendenabled":
		return true
	}
	return false
}

func v1SensitiveConfigurationKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "password") || strings.Contains(key, "secret") || strings.Contains(key, "token") || strings.Contains(key, "privatekey") || strings.Contains(key, "certificate") || key == "authorizationkey"
}

func (s *Server) v1ChargerOperations(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.v1ChargerOperation(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	correlation, idempotency, ok := v1MutationHeaders(w, r)
	if !ok {
		return
	}
	var request v1ChargerOperationRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if idempotency != request.CMSOperationID || !validV1ChargerOperation(request) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid charger operation"})
		return
	}
	validator, ok := s.v1Store.(v1ChargerOperationMappingValidator)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "charger operation mapping validation unavailable"})
		return
	}
	if err := validator.ValidateV1ChargerOperationMapping(r.Context(), request.CPOID, request.CMSChargerID, request.CMSConnectorID, request.ChargerOCPPIdentity, request.OCPPConnectorNumber); err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	op, duplicate, err := s.v1Store.CreateV1ChargerOperation(r.Context(), store.V1ChargerOperationInput{CMSOperationID: request.CMSOperationID, RequestDigest: digestJSON(request, idempotency), CPOID: request.CPOID, CMSChargerID: request.CMSChargerID, CMSConnectorID: request.CMSConnectorID, ChargerOCPPIdentity: request.ChargerOCPPIdentity, OCPPConnectorNumber: request.OCPPConnectorNumber, Kind: request.Kind, Parameters: request.Parameters, CorrelationID: correlation})
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	if !duplicate {
		if _, claimed, claimErr := s.v1Store.ClaimV1ChargerOperationDelivery(r.Context(), request.CMSOperationID); claimErr != nil {
			s.writeV1StoreError(w, claimErr)
			return
		} else if claimed {
			result, dispatchErr := s.dispatchV1ChargerOperation(r.Context(), request)
			state, category := "OCPP_CONFIRMED", ""
			if dispatchErr != nil {
				state, category, result = "RECONCILIATION_REQUIRED", "delivery_ambiguous", ""
			}
			op, err = s.v1Store.MarkV1ChargerOperationDelivery(r.Context(), request.CMSOperationID, state, result, category)
			if err != nil {
				s.writeV1StoreError(w, err)
				return
			}
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"operation": v1ChargerOperationView(op), "correlation_id": correlation, "duplicate": duplicate})
}

func (s *Server) v1ChargerOperation(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("cms_operation_id")
	if !validUUID(id) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "cms_operation_id UUID required"})
		return
	}
	op, err := s.v1Store.GetV1ChargerOperation(r.Context(), id)
	if err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"operation": v1ChargerOperationView(op)})
}

func (s *Server) dispatchV1ChargerOperation(ctx context.Context, request v1ChargerOperationRequest) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	switch request.Kind {
	case "RESET":
		return s.hal.Reset(ctx, request.ChargerOCPPIdentity, strings.ToLower(request.Parameters["type"]))
	case "UNLOCK_CONNECTOR":
		return s.hal.UnlockConnector(ctx, request.ChargerOCPPIdentity, request.OCPPConnectorNumber)
	case "CHANGE_AVAILABILITY":
		return s.hal.ChangeAvailability(ctx, request.ChargerOCPPIdentity, request.OCPPConnectorNumber, strings.ToLower(request.Parameters["type"]))
	case "CLEAR_CACHE":
		return s.hal.ClearCache(ctx, request.ChargerOCPPIdentity)
	case "CHANGE_CONFIGURATION":
		return s.hal.ChangeConfiguration(ctx, request.ChargerOCPPIdentity, request.Parameters["key"], request.Parameters["value"])
	case "TRIGGER_MESSAGE":
		return s.hal.TriggerMessage(ctx, request.ChargerOCPPIdentity, request.Parameters["requested_message"], request.OCPPConnectorNumber)
	default:
		return "", errors.New("unsupported charger operation")
	}
}

func (s *Server) v1ConfigurationRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var request v1ConfigurationReadRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if !validUUID(request.CPOID) || !validUUID(request.CMSChargerID) || strings.TrimSpace(request.ChargerOCPPIdentity) == "" || len(request.Keys) > 64 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid configuration read"})
		return
	}
	for _, key := range request.Keys {
		if !validV1ConfigurationKey(key) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid configuration read"})
			return
		}
	}
	validator, ok := s.v1Store.(v1ChargerOperationMappingValidator)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "charger operation mapping validation unavailable"})
		return
	}
	if err := validator.ValidateV1ChargerOperationMapping(r.Context(), request.CPOID, request.CMSChargerID, "", request.ChargerOCPPIdentity, 0); err != nil {
		s.writeV1StoreError(w, err)
		return
	}
	confirmation, err := s.hal.GetConfiguration(r.Context(), request.ChargerOCPPIdentity, request.Keys)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "charger configuration unavailable"})
		return
	}
	items := make([]map[string]any, 0, len(confirmation.ConfigurationKey))
	for _, item := range confirmation.ConfigurationKey {
		redacted := v1SensitiveConfigurationKey(item.Key)
		value := any(nil)
		if !redacted && item.Value != nil {
			value = *item.Value
		}
		items = append(items, map[string]any{"key": item.Key, "readonly": item.Readonly, "value": value, "redacted": redacted})
	}
	writeJSON(w, http.StatusOK, map[string]any{"configuration_keys": items, "unknown_keys": confirmation.UnknownKey})
}
