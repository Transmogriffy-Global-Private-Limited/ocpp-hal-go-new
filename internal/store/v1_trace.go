package store

import (
	"context"
	"strings"
	"time"
)

// V1Trace is immutable diagnostic evidence. It deliberately does not model or
// decide transaction, connector, billing, or command state.
type V1Trace struct {
	TraceID               string    `json:"trace_id"`
	CPOID                 string    `json:"cpo_id"`
	CMSStartIntentID      string    `json:"cms_start_intent_id,omitempty"`
	CMSChargingSessionID  string    `json:"cms_charging_session_id,omitempty"`
	CMSCommandID          string    `json:"cms_command_id,omitempty"`
	CMSChargerOperationID string    `json:"cms_charger_operation_id,omitempty"`
	HALChargerOperationID string    `json:"hal_charger_operation_id,omitempty"`
	HALTransactionID      string    `json:"hal_transaction_id,omitempty"`
	OCPPTransactionID     *int64    `json:"ocpp_transaction_id,omitempty"`
	ChargerOCPPIdentity   string    `json:"charger_ocpp_identity"`
	OCPPConnectorNumber   int       `json:"ocpp_connector_number"`
	CreatedAt             time.Time `json:"created_at"`
}

type V1TraceEvent struct {
	EventID       string    `json:"event_id"`
	TraceID       string    `json:"trace_id"`
	Source        string    `json:"source"`
	Target        string    `json:"target"`
	Category      string    `json:"category"`
	Protocol      string    `json:"protocol"`
	Phase         string    `json:"phase"`
	Summary       string    `json:"summary"`
	OccurredAt    time.Time `json:"occurred_at"`
	RecordedAt    time.Time `json:"recorded_at"`
	StateBefore   string    `json:"state_before,omitempty"`
	StateAfter    string    `json:"state_after,omitempty"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	Data          any       `json:"data,omitempty"`
}

type V1TraceEventInput struct {
	Source, Target, Category, Protocol, Phase, Summary string
	OccurredAt                                         time.Time
	StateBefore, StateAfter, CorrelationID             string
	Data                                               any
}

// sanitizeV1TraceData is the final persistence boundary for diagnostic data.
// Call sites may annotate evidence, but unsupported fields (in particular
// idTags, credentials, authorization material and raw OCPP payloads) cannot
// become durable trace data by accident.
func sanitizeV1TraceData(data any) map[string]any {
	input, ok := data.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	if _, operationEvidence := input["message_type"]; operationEvidence {
		return sanitizeV1OperationEvidence(input)
	}
	output := make(map[string]any, len(input))
	for _, key := range []string{"action", "result", "status", "transaction_id", "connector_id", "meter_wh", "reason", "error_class", "unique_id", "message_type", "direction", "payload"} {
		if value, exists := input[key]; exists {
			output[key] = value
		}
	}
	return output
}

// sanitizeV1OperationEvidence is deliberately repeated at the durable HAL
// boundary. The observer already constructs safe data, but no caller may
// make raw OCPP payloads persistent by bypassing that helper.
func sanitizeV1OperationEvidence(input map[string]any) map[string]any {
	uniqueID, uniqueOK := v1SafeText(input["unique_id"], 128)
	action, actionOK := v1SafeText(input["action"], 64)
	messageType, typeOK := input["message_type"].(string)
	payload, payloadOK := input["payload"].(map[string]any)
	if !uniqueOK || !actionOK || !typeOK || !payloadOK || !v1OperationAction(action) || (messageType != "CALL" && messageType != "CALLRESULT" && messageType != "CALLERROR") {
		return map[string]any{}
	}
	safePayload, ok := v1SafeOperationPayload(action, messageType, payload)
	if !ok {
		return map[string]any{}
	}
	return map[string]any{"unique_id": uniqueID, "action": action, "message_type": messageType, "payload": safePayload}
}

func v1OperationAction(action string) bool {
	switch action {
	case "Reset", "UnlockConnector", "ChangeAvailability", "ClearCache", "ChangeConfiguration", "TriggerMessage", "GetConfiguration":
		return true
	}
	return false
}
func v1SafeText(value any, maximum int) (string, bool) {
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	return text, ok && text != "" && len(text) <= maximum
}
func v1SafeConnector(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, number >= 0 && number <= 999
	case float64:
		return int(number), number >= 0 && number <= 999 && number == float64(int(number))
	}
	return 0, false
}
func v1SafeOperationPayload(action, messageType string, payload map[string]any) (map[string]any, bool) {
	if messageType == "CALLERROR" {
		code, ok := v1SafeText(payload["error_code"], 64)
		return map[string]any{"error_code": code}, ok && len(payload) == 1
	}
	if messageType == "CALLRESULT" {
		if action == "GetConfiguration" {
			return v1SafeConfigurationResult(payload)
		}
		status, ok := v1SafeText(payload["status"], 64)
		return map[string]any{"status": status}, ok && len(payload) == 1
	}
	switch action {
	case "Reset":
		value, ok := v1SafeText(payload["type"], 16)
		return map[string]any{"type": value}, ok && (value == "Soft" || value == "Hard") && len(payload) == 1
	case "UnlockConnector":
		connector, ok := v1SafeConnector(payload["connectorId"])
		return map[string]any{"connectorId": connector}, ok && len(payload) == 1
	case "ChangeAvailability":
		connector, connectorOK := v1SafeConnector(payload["connectorId"])
		value, valueOK := v1SafeText(payload["type"], 16)
		return map[string]any{"connectorId": connector, "type": value}, connectorOK && valueOK && (value == "Operative" || value == "Inoperative") && len(payload) == 2
	case "ClearCache":
		return map[string]any{}, len(payload) == 0
	case "ChangeConfiguration":
		key, keyOK := v1SafeText(payload["key"], 100)
		redacted, redactedOK := payload["redacted"].(bool)
		return map[string]any{"key": key, "redacted": true}, keyOK && redactedOK && redacted && len(payload) == 2
	case "TriggerMessage":
		message, messageOK := v1SafeText(payload["requestedMessage"], 64)
		safe := map[string]any{"requestedMessage": message}
		if connector, exists := payload["connectorId"]; exists {
			number, ok := v1SafeConnector(connector)
			if !ok {
				return nil, false
			}
			safe["connectorId"] = number
		}
		return safe, messageOK && (len(payload) == 1 || len(payload) == 2)
	case "GetConfiguration":
		keys, ok := v1SafeStringList(payload["configuration_keys"], 64, 100)
		return map[string]any{"configuration_keys": keys}, ok && len(payload) == 1
	}
	return nil, false
}
func v1SafeConfigurationResult(payload map[string]any) (map[string]any, bool) {
	items, ok := payload["configuration_keys"].([]any)
	if !ok || len(items) > 64 {
		return nil, false
	}
	keys := make([]map[string]any, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || len(item) < 2 || len(item) > 3 {
			return nil, false
		}
		key, keyOK := v1SafeText(item["key"], 100)
		redacted, redactedOK := item["redacted"].(bool)
		if !keyOK || !redactedOK {
			return nil, false
		}
		safe := map[string]any{"key": key, "redacted": redacted}
		if value, exists := item["value"]; exists && !redacted {
			text, valid := value.(string)
			if !valid || len(text) > 500 {
				return nil, false
			}
			safe["value"] = text
		} else if exists {
			return nil, false
		}
		keys = append(keys, safe)
	}
	unknown, unknownOK := v1SafeStringList(payload["unknown_keys"], 64, 100)
	if !unknownOK || len(payload) != 2 {
		return nil, false
	}
	return map[string]any{"configuration_keys": keys, "unknown_keys": unknown}, true
}
func v1SafeStringList(value any, maximum, itemMaximum int) ([]string, bool) {
	var items []any
	switch list := value.(type) {
	case []any:
		items = list
	case []string:
		items = make([]any, len(list))
		for i := range list {
			items[i] = list[i]
		}
	default:
		return nil, false
	}
	if len(items) > maximum {
		return nil, false
	}
	output := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := v1SafeText(item, itemMaximum)
		if !ok {
			return nil, false
		}
		output = append(output, text)
	}
	return output, true
}

// V1TraceStore is intentionally separate from V1Store so a diagnostic
// persistence failure can never alter OCPP acknowledgement semantics or break
// existing focused test doubles.
type V1TraceStore interface {
	EnsureV1Trace(context.Context, V1Trace) (*V1Trace, error)
	BindV1TraceTransaction(context.Context, string, *V1Transaction) error
	EnsureV1TraceForTransaction(context.Context, *V1Transaction) (*V1Trace, error)
	FindV1TraceByTransaction(context.Context, string) (*V1Trace, error)
	FindV1TraceForConnector(context.Context, string, int) (*V1Trace, error)
	AppendV1TraceEvent(context.Context, string, V1TraceEventInput) error
	GetV1Trace(context.Context, string) (*V1Trace, error)
	ListV1TraceEvents(context.Context, string, time.Time, string, int) ([]V1TraceEvent, error)
	DeleteV1TracesBefore(context.Context, time.Time, int) (int64, error)
}
