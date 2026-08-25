package httpapi

import (
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

// These views are the complete HAL v1 HTTP response boundary. Store records
// deliberately stay untagged: they carry persistence and delivery internals
// that must not become public merely because a handler returns them.

type v1CommandResponse struct {
	HALCommandID      string    `json:"hal_command_id"`
	CMSCommandID      string    `json:"cms_command_id"`
	Kind              string    `json:"kind"`
	State             string    `json:"state"`
	HALTransactionID  *string   `json:"hal_transaction_id"`
	OCPPTransactionID *int64    `json:"ocpp_transaction_id"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func v1CommandView(command *store.V1RemoteCommand) v1CommandResponse {
	response := v1CommandResponse{
		HALCommandID:      command.HALCommandID,
		CMSCommandID:      command.CMSCommandID,
		Kind:              command.Kind,
		State:             command.State,
		OCPPTransactionID: command.OCPPTransactionID,
		UpdatedAt:         command.UpdatedAt,
	}
	if command.HALTransactionID != "" {
		response.HALTransactionID = &command.HALTransactionID
	}
	return response
}

type v1ConnectorMappingResponse struct {
	CPOID               string `json:"cpo_id"`
	CMSChargerID        string `json:"cms_charger_id"`
	CMSConnectorID      string `json:"cms_connector_id"`
	ChargerOCPPIdentity string `json:"charger_ocpp_identity"`
	OCPPConnectorNumber int    `json:"ocpp_connector_number"`
}

type v1MappingResponse struct {
	CPOID               string                       `json:"cpo_id"`
	CMSChargerID        string                       `json:"cms_charger_id"`
	ChargerOCPPIdentity string                       `json:"charger_ocpp_identity"`
	ExpectedSerial      string                       `json:"expected_serial,omitempty"`
	Enabled             bool                         `json:"enabled"`
	Connectors          []v1ConnectorMappingResponse `json:"connectors"`
}

func v1MappingView(mapping *store.V1ChargerMapping) v1MappingResponse {
	connectors := make([]v1ConnectorMappingResponse, 0, len(mapping.Connectors))
	for _, connector := range mapping.Connectors {
		connectors = append(connectors, v1ConnectorMappingResponse{
			CPOID:               connector.CPOID,
			CMSChargerID:        connector.CMSChargerID,
			CMSConnectorID:      connector.CMSConnectorID,
			ChargerOCPPIdentity: connector.ChargerOCPPIdentity,
			OCPPConnectorNumber: connector.OCPPConnectorNumber,
		})
	}
	return v1MappingResponse{CPOID: mapping.CPOID, CMSChargerID: mapping.CMSChargerID, ChargerOCPPIdentity: mapping.ChargerOCPPIdentity, ExpectedSerial: mapping.ExpectedSerial, Enabled: mapping.Enabled, Connectors: connectors}
}

type v1StopWorkflowResponse struct {
	HALTransactionID       string     `json:"hal_transaction_id"`
	RequestedStopInitiator string     `json:"requested_stop_initiator"`
	RequestedStopReason    string     `json:"requested_stop_reason"`
	State                  string     `json:"state"`
	DeliveryAttempts       int        `json:"delivery_attempts"`
	LastOCPPResult         string     `json:"last_ocpp_result"`
	LastErrorCategory      string     `json:"last_error_category"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	CompletedAt            *time.Time `json:"completed_at"`
}

func v1StopWorkflowView(workflow *store.V1StopWorkflow) *v1StopWorkflowResponse {
	if workflow == nil {
		return nil
	}
	return &v1StopWorkflowResponse{HALTransactionID: workflow.HALTransactionID, RequestedStopInitiator: workflow.RequestedStopInitiator, RequestedStopReason: workflow.RequestedStopReason, State: workflow.State, DeliveryAttempts: workflow.DeliveryAttempts, LastOCPPResult: workflow.LastOCPPResult, LastErrorCategory: workflow.LastErrorCategory, CreatedAt: workflow.CreatedAt, UpdatedAt: workflow.UpdatedAt, CompletedAt: workflow.CompletedAt}
}

type v1TransactionResponse struct {
	HALTransactionID       string              `json:"hal_transaction_id"`
	CMSStartIntentID       string              `json:"cms_start_intent_id"`
	CMSCommandID           string              `json:"cms_command_id"`
	CPOID                  string              `json:"cpo_id"`
	CMSChargerID           string              `json:"cms_charger_id"`
	CMSConnectorID         string              `json:"cms_connector_id"`
	ChargerOCPPIdentity    string              `json:"charger_ocpp_identity"`
	OCPPConnectorNumber    int                 `json:"ocpp_connector_number"`
	OCPPTransactionID      int64               `json:"ocpp_transaction_id"`
	ActualStartedAt        time.Time           `json:"actual_started_at"`
	ObservedStartedAt      time.Time           `json:"observed_started_at"`
	MeterStartWh           int64               `json:"meter_start_wh"`
	LatestMeterWh          *int64              `json:"latest_meter_wh,omitempty"`
	ConsumedWh             *int64              `json:"consumed_wh,omitempty"`
	MeterObservedAt        *time.Time          `json:"meter_observed_at,omitempty"`
	MeterSequence          int64               `json:"meter_sequence"`
	InitialSoCPercent      *store.V1SoCPercent `json:"initial_soc_percent,omitempty"`
	LatestSoCPercent       *store.V1SoCPercent `json:"latest_soc_percent,omitempty"`
	SoCObservedAt          *time.Time          `json:"soc_observed_at,omitempty"`
	SoCSequence            int64               `json:"soc_sequence"`
	EnergyLimitWh          *int64              `json:"energy_limit_wh,omitempty"`
	MaxDurationSeconds     *int64              `json:"max_duration_seconds,omitempty"`
	StopDeadlineAt         *time.Time          `json:"stop_deadline_at,omitempty"`
	StopState              string              `json:"stop_state"`
	RequestedStopInitiator string              `json:"requested_stop_initiator,omitempty"`
	RequestedStopReason    string              `json:"requested_stop_reason,omitempty"`
	OCPPStopReason         string              `json:"ocpp_stop_reason,omitempty"`
	CompletedAt            *time.Time          `json:"completed_at,omitempty"`
	ObservedCompletedAt    *time.Time          `json:"observed_completed_at,omitempty"`
	MeterStopWh            *int64              `json:"meter_stop_wh,omitempty"`
}

func v1TransactionView(transaction *store.V1Transaction) v1TransactionResponse {
	return v1TransactionResponse{HALTransactionID: transaction.HALTransactionID, CMSStartIntentID: transaction.CMSStartIntentID, CMSCommandID: transaction.CMSCommandID, CPOID: transaction.CPOID, CMSChargerID: transaction.CMSChargerID, CMSConnectorID: transaction.CMSConnectorID, ChargerOCPPIdentity: transaction.ChargerOCPPIdentity, OCPPConnectorNumber: transaction.OCPPConnectorNumber, OCPPTransactionID: transaction.OCPPTransactionID, ActualStartedAt: transaction.ActualStartedAt, ObservedStartedAt: transaction.ObservedStartedAt, MeterStartWh: transaction.MeterStartWh, LatestMeterWh: transaction.LatestMeterWh, ConsumedWh: transaction.ConsumedWh, MeterObservedAt: transaction.MeterObservedAt, MeterSequence: transaction.MeterSequence, InitialSoCPercent: transaction.InitialSoCPercent, LatestSoCPercent: transaction.LatestSoCPercent, SoCObservedAt: transaction.SoCObservedAt, SoCSequence: transaction.SoCSequence, EnergyLimitWh: transaction.EnergyLimitWh, MaxDurationSeconds: transaction.MaxDurationSeconds, StopDeadlineAt: transaction.StopDeadlineAt, StopState: transaction.StopState, RequestedStopInitiator: transaction.RequestedStopInitiator, RequestedStopReason: transaction.RequestedStopReason, OCPPStopReason: transaction.OCPPStopReason, CompletedAt: transaction.CompletedAt, ObservedCompletedAt: transaction.ObservedCompletedAt, MeterStopWh: transaction.MeterStopWh}
}

type v1ConnectorRuntimeResponse struct {
	CPOID               string     `json:"cpo_id"`
	CMSChargerID        string     `json:"cms_charger_id"`
	CMSConnectorID      string     `json:"cms_connector_id"`
	ChargerOCPPIdentity string     `json:"charger_ocpp_identity"`
	OCPPConnectorNumber int        `json:"ocpp_connector_number"`
	Status              string     `json:"status"`
	ErrorCode           string     `json:"error_code"`
	Info                string     `json:"info"`
	VendorID            string     `json:"vendor_id"`
	VendorErrorCode     string     `json:"vendor_error_code"`
	ObservedAt          *time.Time `json:"observed_at"`
	StatusSequence      int64      `json:"status_sequence"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func v1ConnectorRuntimeView(runtime store.V1ConnectorRuntime) v1ConnectorRuntimeResponse {
	return v1ConnectorRuntimeResponse{CPOID: runtime.CPOID, CMSChargerID: runtime.CMSChargerID, CMSConnectorID: runtime.CMSConnectorID, ChargerOCPPIdentity: runtime.ChargerOCPPIdentity, OCPPConnectorNumber: runtime.OCPPConnectorNumber, Status: runtime.Status, ErrorCode: runtime.ErrorCode, Info: runtime.Info, VendorID: runtime.VendorID, VendorErrorCode: runtime.VendorErrorCode, ObservedAt: runtime.ObservedAt, StatusSequence: runtime.StatusSequence, UpdatedAt: runtime.UpdatedAt}
}

type v1ChargerRuntimeResponse struct {
	CPOID                string                       `json:"cpo_id"`
	CMSChargerID         string                       `json:"cms_charger_id"`
	ChargerOCPPIdentity  string                       `json:"charger_ocpp_identity"`
	ConnectionState      string                       `json:"connection_state"`
	ConnectionGeneration int64                        `json:"connection_generation"`
	ConnectionSequence   int64                        `json:"connection_sequence"`
	ConnectedAt          *time.Time                   `json:"connected_at"`
	LastObservedAt       *time.Time                   `json:"last_observed_at"`
	UpdatedAt            time.Time                    `json:"updated_at"`
	Connectors           []v1ConnectorRuntimeResponse `json:"connectors"`
}

func v1ChargerRuntimeView(runtime *store.V1ChargerRuntime) v1ChargerRuntimeResponse {
	connectors := make([]v1ConnectorRuntimeResponse, 0, len(runtime.Connectors))
	for _, connector := range runtime.Connectors {
		connectors = append(connectors, v1ConnectorRuntimeView(connector))
	}
	return v1ChargerRuntimeResponse{CPOID: runtime.CPOID, CMSChargerID: runtime.CMSChargerID, ChargerOCPPIdentity: runtime.ChargerOCPPIdentity, ConnectionState: runtime.ConnectionState, ConnectionGeneration: runtime.ConnectionGeneration, ConnectionSequence: runtime.ConnectionSequence, ConnectedAt: runtime.ConnectedAt, LastObservedAt: runtime.LastObservedAt, UpdatedAt: runtime.UpdatedAt, Connectors: connectors}
}
