package httpapi

import (
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
	"time"
)

type v1ChargerOperationResponse struct {
	HALOperationID      string     `json:"hal_operation_id"`
	CMSOperationID      string     `json:"cms_operation_id"`
	CPOID               string     `json:"cpo_id"`
	CMSChargerID        string     `json:"cms_charger_id"`
	CMSConnectorID      string     `json:"cms_connector_id,omitempty"`
	OCPPConnectorNumber int        `json:"ocpp_connector_number"`
	Kind                string     `json:"kind"`
	State               string     `json:"state"`
	OCPPResult          string     `json:"ocpp_result,omitempty"`
	ErrorCategory       string     `json:"error_category,omitempty"`
	DeliveryAttempts    int        `json:"delivery_attempts"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
}

func v1ChargerOperationView(operation *store.V1ChargerOperation) v1ChargerOperationResponse {
	return v1ChargerOperationResponse{HALOperationID: operation.HALOperationID, CMSOperationID: operation.CMSOperationID, CPOID: operation.CPOID, CMSChargerID: operation.CMSChargerID, CMSConnectorID: operation.CMSConnectorID, OCPPConnectorNumber: operation.OCPPConnectorNumber, Kind: operation.Kind, State: operation.State, OCPPResult: operation.OCPPResult, ErrorCategory: operation.ErrorCategory, DeliveryAttempts: operation.DeliveryAttempts, CreatedAt: operation.CreatedAt, UpdatedAt: operation.UpdatedAt, CompletedAt: operation.CompletedAt}
}
