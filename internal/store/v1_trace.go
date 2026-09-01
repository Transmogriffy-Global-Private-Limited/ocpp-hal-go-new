package store

import (
	"context"
	"time"
)

// V1Trace is immutable diagnostic evidence. It deliberately does not model or
// decide transaction, connector, billing, or command state.
type V1Trace struct {
	TraceID              string    `json:"trace_id"`
	CPOID                string    `json:"cpo_id"`
	CMSStartIntentID     string    `json:"cms_start_intent_id,omitempty"`
	CMSChargingSessionID string    `json:"cms_charging_session_id,omitempty"`
	CMSCommandID         string    `json:"cms_command_id,omitempty"`
	HALTransactionID     string    `json:"hal_transaction_id,omitempty"`
	OCPPTransactionID    *int64    `json:"ocpp_transaction_id,omitempty"`
	ChargerOCPPIdentity  string    `json:"charger_ocpp_identity"`
	OCPPConnectorNumber  int       `json:"ocpp_connector_number"`
	CreatedAt            time.Time `json:"created_at"`
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
	output := make(map[string]any, len(input))
	for _, key := range []string{"action", "result", "status", "transaction_id", "connector_id", "meter_wh", "reason", "error_class"} {
		if value, exists := input[key]; exists {
			output[key] = value
		}
	}
	return output
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
