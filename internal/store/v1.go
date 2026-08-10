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
	HALTransactionID       string
	CMSStartIntentID       string
	CMSCommandID           string
	CPOID                  string
	CustomerID             string
	CMSChargerID           string
	CMSConnectorID         string
	ChargerOCPPIdentity    string
	OCPPConnectorNumber    int
	IDTag                  string
	OCPPTransactionID      int64
	ActualStartedAt        time.Time
	MeterStartWh           int64
	LatestMeterWh          *int64
	MeterObservedAt        *time.Time
	MeterSequence          int64
	EnergyLimitWh          *int64
	MaxDurationSeconds     *int64
	StopDeadlineAt         *time.Time
	StopState              string
	RequestedStopInitiator string
	RequestedStopReason    string
	OCPPStopReason         string
	CompletedAt            *time.Time
	MeterStopWh            *int64
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
	CreateV1StartCommand(context.Context, V1StartCommandInput) (*V1RemoteCommand, bool, error)
	CreateV1StopCommand(context.Context, V1StopCommandInput) (*V1RemoteCommand, bool, error)
	GetV1Command(context.Context, string) (*V1RemoteCommand, error)
	GetV1Credential(context.Context, string) (*V1Credential, error)
	MaterializeV1Start(context.Context, V1StartMaterialization) (*V1Transaction, bool, error)
	UpdateV1Meter(context.Context, string, int64, int64, time.Time) (*V1Transaction, error)
	RequestV1Stop(context.Context, string, string, string) (*V1Transaction, bool, error)
	CompleteV1Transaction(context.Context, string, int64, string, time.Time) (*V1Transaction, error)
	GetV1Transaction(context.Context, string) (*V1Transaction, error)
	GetV1TransactionByStartIntent(context.Context, string) (*V1Transaction, error)
	AuthorizeV1Credential(context.Context, string, string, time.Time) error
	ClaimV1StartDelivery(context.Context, string) (*V1RemoteCommand, bool, error)
	MarkV1CommandDelivery(context.Context, string, string, string, string) (*V1RemoteCommand, error)
	RecordV1ChargerConnection(context.Context, string, int64, bool, time.Time) error
	RecordV1ConnectorStatus(context.Context, V1ConnectorRuntime) error
	GetV1ChargerRuntime(context.Context, string) (*V1ChargerRuntime, error)
	GetV1ConnectorRuntime(context.Context, string) (*V1ConnectorRuntime, error)
	ResetV1ConnectionRuntime(context.Context) error
}
