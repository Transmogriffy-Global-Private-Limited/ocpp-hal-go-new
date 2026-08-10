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
)

type V1StartCommandInput struct {
	CMSCommandID        string
	RequestDigest       string
	CPOID               string
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

type V1StopCommandInput struct {
	CMSCommandID           string
	RequestDigest          string
	CPOID                  string
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

type V1StartMaterialization struct {
	ChargerOCPPIdentity string
	OCPPConnectorNumber int
	IDTag               string
	MeterStartWh        int64
	ActualStartedAt     time.Time
	OCPPTransactionID   int64
}

type V1Store interface {
	CreateV1StartCommand(context.Context, V1StartCommandInput) (*V1RemoteCommand, bool, error)
	CreateV1StopCommand(context.Context, V1StopCommandInput) (*V1RemoteCommand, bool, error)
	GetV1Command(context.Context, string) (*V1RemoteCommand, error)
	GetV1Credential(context.Context, string) (*V1Credential, error)
	MaterializeV1Start(context.Context, V1StartMaterialization) (*V1Transaction, bool, error)
	UpdateV1Meter(context.Context, string, int64, int64, time.Time) (*V1Transaction, error)
	RequestV1Stop(context.Context, string, string, string) (*V1Transaction, bool, error)
	CompleteV1Transaction(context.Context, string, int64, string, time.Time) (*V1Transaction, error)
	GetV1Transaction(context.Context, string) (*V1Transaction, error)
}
