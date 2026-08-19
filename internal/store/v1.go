package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrV1CommandNotFound     = errors.New("v1 remote command not found")
	ErrV1CredentialRejected  = errors.New("v1 start credential rejected")
	ErrV1TransactionNotFound = errors.New("v1 transaction not found")
	ErrV1IdempotencyConflict = errors.New("v1 command idempotency conflict")
	ErrV1MappingNotFound     = errors.New("v1 mapping not found")
	ErrV1MappingConflict     = errors.New("v1 mapping conflict")
	ErrV1DeliveryNotReady    = errors.New("v1 delivery is not ready")
)

type V1StartCommandInput struct {
	CMSCommandID        string
	RequestDigest       string
	CPOID               string
	CustomerID          string
	CorrelationID       string
	CMSStartIntentID    string
	CMSChargerID        string
	CMSConnectorID      string
	ChargerOCPPIdentity string
	OCPPConnectorNumber int
	IDTag               string
	CredentialExpiresAt time.Time
	CommandExpiresAt    time.Time
	EnergyLimitWh       *int64
	MaxDurationSeconds  *int64
}

type V1MappingInput struct {
	CPOID               string
	CMSChargerID        string
	ChargerOCPPIdentity string
	Enabled             bool
	Connectors          []V1ConnectorMappingInput
	CorrelationID       string
	RequestDigest       string
}

type V1ConnectorMappingInput struct {
	CMSConnectorID      string
	OCPPConnectorNumber int
}

type V1ChargerMapping struct {
	CPOID               string
	CMSChargerID        string
	ChargerOCPPIdentity string
	Enabled             bool
	Connectors          []V1ConnectorMapping
}

type V1ConnectorMapping struct {
	CPOID               string
	CMSChargerID        string
	CMSConnectorID      string
	ChargerOCPPIdentity string
	OCPPConnectorNumber int
}

type V1StopCommandInput struct {
	CMSCommandID           string
	RequestDigest          string
	CPOID                  string
	CustomerID             string
	CorrelationID          string
	CMSChargingSessionID   string
	CMSChargerID           string
	CMSConnectorID         string
	ChargerOCPPIdentity    string
	OCPPConnectorNumber    int
	HALTransactionID       string
	OCPPTransactionID      int64
	RequestedStopInitiator string
	RequestedStopReason    string
	CommandExpiresAt       time.Time
}

type V1RemoteCommand struct {
	HALCommandID           string
	CMSCommandID           string
	Kind                   string
	RequestDigest          string
	CPOID                  string
	CustomerID             string
	CorrelationID          string
	CMSStartIntentID       string
	CMSChargingSessionID   string
	CMSChargerID           string
	CMSConnectorID         string
	ChargerOCPPIdentity    string
	OCPPConnectorNumber    int
	IDTag                  string
	CredentialExpiresAt    *time.Time
	CommandExpiresAt       time.Time
	EnergyLimitWh          *int64
	MaxDurationSeconds     *int64
	RequestedStopInitiator string
	RequestedStopReason    string
	HALTransactionID       string
	OCPPTransactionID      *int64
	State                  string
	DeliveryAttempts       int
	LastOCPPResult         string
	LastErrorCategory      string
	LastErrorDetail        string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type V1Credential struct {
	IDTag               string
	CMSStartIntentID    string
	CPOID               string
	CMSChargerID        string
	CMSConnectorID      string
	ChargerOCPPIdentity string
	OCPPConnectorNumber int
	ExpiresAt           time.Time
	ConsumedAt          *time.Time
	HALTransactionID    string
}

type V1Transaction struct {
	HALTransactionID       string     `json:"hal_transaction_id"`
	CMSStartIntentID       string     `json:"cms_start_intent_id"`
	CMSCommandID           string     `json:"cms_command_id"`
	CPOID                  string     `json:"cpo_id"`
	CustomerID             string     `json:"customer_id"`
	CMSChargerID           string     `json:"cms_charger_id"`
	CMSConnectorID         string     `json:"cms_connector_id"`
	ChargerOCPPIdentity    string     `json:"charger_ocpp_identity"`
	OCPPConnectorNumber    int        `json:"ocpp_connector_number"`
	IDTag                  string     `json:"id_tag"`
	OCPPTransactionID      int64      `json:"ocpp_transaction_id"`
	ActualStartedAt        time.Time  `json:"actual_started_at"`
	MeterStartWh           int64      `json:"meter_start_wh"`
	LatestMeterWh          *int64     `json:"latest_meter_wh,omitempty"`
	ConsumedWh             *int64     `json:"consumed_wh,omitempty"`
	MeterObservedAt        *time.Time `json:"meter_observed_at,omitempty"`
	MeterSequence          int64      `json:"meter_sequence"`
	EnergyLimitWh          *int64     `json:"energy_limit_wh,omitempty"`
	MaxDurationSeconds     *int64     `json:"max_duration_seconds,omitempty"`
	StopDeadlineAt         *time.Time `json:"stop_deadline_at,omitempty"`
	StopState              string     `json:"stop_state"`
	RequestedStopInitiator string     `json:"requested_stop_initiator,omitempty"`
	RequestedStopReason    string     `json:"requested_stop_reason,omitempty"`
	OCPPStopReason         string     `json:"ocpp_stop_reason,omitempty"`
	CompletedAt            *time.Time `json:"completed_at,omitempty"`
	MeterStopWh            *int64     `json:"meter_stop_wh,omitempty"`
}

type V1StopWorkflow struct {
	HALTransactionID       string
	RequestedStopInitiator string
	RequestedStopReason    string
	State                  string
	DeliveryAttempts       int
	LastOCPPResult         string
	LastErrorCategory      string
	LastErrorDetail        string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	CompletedAt            *time.Time
}

type V1Fact struct {
	FactID         string
	FactType       string
	SchemaVersion  int
	OccurredAt     time.Time
	Producer       string
	ContentSHA256  string
	Payload        []byte
	Status         string
	Retries        int
	NextRetryAt    time.Time
	ClaimedUntil   *time.Time
	DeliveryStatus *int
	LastError      string
}

type V1ChargerRuntime struct {
	CPOID                string
	CMSChargerID         string
	ChargerOCPPIdentity  string
	ConnectionState      string
	ConnectionGeneration int64
	ConnectionSequence   int64
	ConnectedAt          *time.Time
	LastObservedAt       *time.Time
	UpdatedAt            time.Time
	Connectors           []V1ConnectorRuntime
}

type V1ConnectorRuntime struct {
	CPOID               string
	CMSChargerID        string
	CMSConnectorID      string
	ChargerOCPPIdentity string
	OCPPConnectorNumber int
	Status              string
	ErrorCode           string
	Info                string
	VendorID            string
	VendorErrorCode     string
	ObservedAt          *time.Time
	StatusSequence      int64
	UpdatedAt           time.Time
}

type V1StartMaterialization struct {
	ChargerOCPPIdentity string
	OCPPConnectorNumber int
	IDTag               string
	MeterStartWh        int64
	ActualStartedAt     time.Time
	OCPPTransactionID   int64
}

type V1Store interface {
	SyncV1Mapping(context.Context, V1MappingInput) (*V1ChargerMapping, bool, error)
	ValidateV1Mapping(context.Context, string, string, string, string, int) error
	ValidateV1ChargerAdmission(context.Context, string) error
	CreateV1StartCommand(context.Context, V1StartCommandInput) (*V1RemoteCommand, bool, error)
	CreateV1StopCommand(context.Context, V1StopCommandInput) (*V1RemoteCommand, bool, error)
	GetV1Command(context.Context, string) (*V1RemoteCommand, error)
	GetV1Credential(context.Context, string) (*V1Credential, error)
	MaterializeV1Start(context.Context, V1StartMaterialization) (*V1Transaction, bool, error)
	UpdateV1Meter(context.Context, string, int64, int64, time.Time) (*V1Transaction, error)
	UpdateV1MeterForOCPP(context.Context, string, int64, int64, time.Time) (*V1Transaction, bool, error)
	RequestV1Stop(context.Context, string, string, string) (*V1Transaction, bool, error)
	CompleteV1Transaction(context.Context, string, int64, string, time.Time) (*V1Transaction, error)
	GetV1Transaction(context.Context, string) (*V1Transaction, error)
	GetV1TransactionByStartIntent(context.Context, string) (*V1Transaction, error)
	GetV1TransactionByOCPP(context.Context, string, int64) (*V1Transaction, error)
	AuthorizeV1Credential(context.Context, string, string, time.Time) error
	ClaimV1StartDelivery(context.Context, string) (*V1RemoteCommand, bool, error)
	BeginV1CommandDelivery(context.Context, string) (*V1RemoteCommand, error)
	MarkV1CommandDelivery(context.Context, string, string, string, string) (*V1RemoteCommand, error)
	RecoverV1CommandDelivery(context.Context) error
	EnsureV1StopWorkflow(context.Context, string, string, string) (*V1StopWorkflow, bool, error)
	GetV1StopWorkflow(context.Context, string) (*V1StopWorkflow, error)
	ClaimV1StopDelivery(context.Context, string) (*V1StopWorkflow, bool, error)
	BeginV1StopDelivery(context.Context, string) (*V1StopWorkflow, error)
	MarkV1StopDelivery(context.Context, string, string, string, string) (*V1StopWorkflow, error)
	RecoverV1StopDelivery(context.Context) error
	ListV1OverdueTransactions(context.Context, time.Time, int) ([]*V1Transaction, error)
	ClaimV1Facts(context.Context, time.Time, int) ([]V1Fact, error)
	MarkV1FactDelivery(context.Context, string, int, bool, bool, string, time.Time) error
	RecordV1ChargerConnection(context.Context, string, int64, bool, time.Time) error
	RenewCurrentV1ChargerConnection(context.Context, string, int64, time.Time) error
	RecordV1ConnectorStatus(context.Context, V1ConnectorRuntime) error
	GetV1ChargerRuntime(context.Context, string) (*V1ChargerRuntime, error)
	GetV1ConnectorRuntime(context.Context, string) (*V1ConnectorRuntime, error)
	ResetV1ConnectionRuntime(context.Context) error
}
