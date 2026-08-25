package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

var (
	ErrV1CommandNotFound      = errors.New("v1 remote command not found")
	ErrV1CredentialRejected   = errors.New("v1 start credential rejected")
	ErrV1TransactionNotFound  = errors.New("v1 transaction not found")
	ErrV1IdempotencyConflict  = errors.New("v1 command idempotency conflict")
	ErrV1MappingNotFound      = errors.New("v1 mapping not found")
	ErrV1MappingConflict      = errors.New("v1 mapping conflict")
	ErrV1IdentityConflict     = errors.New("v1 charger identity evidence conflicts with mapping")
	ErrV1DeliveryNotReady     = errors.New("v1 delivery is not ready")
	ErrV1InvalidEvidence      = errors.New("v1 transaction evidence is invalid")
	ErrV1FactNotFound         = errors.New("v1 fact not found")
	ErrV1FactNotReconciliable = errors.New("v1 fact is not awaiting reconciliation")
	ErrV1FactClaimLost        = errors.New("v1 fact delivery claim is no longer current")
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
	ExpectedSerial      string
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
	ExpectedSerial      string
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
	HALTransactionID              string        `json:"hal_transaction_id"`
	CMSStartIntentID              string        `json:"cms_start_intent_id"`
	CMSCommandID                  string        `json:"cms_command_id"`
	CPOID                         string        `json:"cpo_id"`
	CustomerID                    string        `json:"customer_id"`
	CMSChargerID                  string        `json:"cms_charger_id"`
	CMSConnectorID                string        `json:"cms_connector_id"`
	ChargerOCPPIdentity           string        `json:"charger_ocpp_identity"`
	OCPPConnectorNumber           int           `json:"ocpp_connector_number"`
	IDTag                         string        `json:"id_tag"`
	OCPPTransactionID             int64         `json:"ocpp_transaction_id"`
	ActualStartedAt               time.Time     `json:"actual_started_at"`
	ObservedStartedAt             time.Time     `json:"observed_started_at"`
	MeterStartWh                  int64         `json:"meter_start_wh"`
	LatestMeterWh                 *int64        `json:"latest_meter_wh,omitempty"`
	ConsumedWh                    *int64        `json:"consumed_wh,omitempty"`
	MeterObservedAt               *time.Time    `json:"meter_observed_at,omitempty"`
	MeterSequence                 int64         `json:"meter_sequence"`
	InitialSoCPercent             *V1SoCPercent `json:"initial_soc_percent,omitempty"`
	LatestSoCPercent              *V1SoCPercent `json:"latest_soc_percent,omitempty"`
	SoCObservedAt                 *time.Time    `json:"soc_observed_at,omitempty"`
	SoCSequence                   int64         `json:"soc_sequence"`
	EnergyLimitWh                 *int64        `json:"energy_limit_wh,omitempty"`
	MaxDurationSeconds            *int64        `json:"max_duration_seconds,omitempty"`
	StopDeadlineAt                *time.Time    `json:"stop_deadline_at,omitempty"`
	StopState                     string        `json:"stop_state"`
	RequestedStopInitiator        string        `json:"requested_stop_initiator,omitempty"`
	RequestedStopReason           string        `json:"requested_stop_reason,omitempty"`
	OCPPStopReason                string        `json:"ocpp_stop_reason,omitempty"`
	CompletedAt                   *time.Time    `json:"completed_at,omitempty"`
	ObservedCompletedAt           *time.Time    `json:"observed_completed_at,omitempty"`
	MeterStopWh                   *int64        `json:"meter_stop_wh,omitempty"`
	RawMeterStopWh                *int64        `json:"raw_meter_stop_wh,omitempty"`
	MeterStopAdjustmentWh         *int64        `json:"meter_stop_adjustment_wh,omitempty"`
	MeterStopEvidence             string        `json:"meter_stop_evidence,omitempty"`
	MeterQuantizationAnomalyCount int64         `json:"meter_quantization_anomaly_count"`
}

// V1SoCPercent stores charger evidence in thousandths of a percent. The
// integer representation keeps ordering/durability exact while JSON exposes a
// conventional decimal percentage (for example, 63500 becomes 63.5).
type V1SoCPercent int64

const v1SoCScale = int64(1000)

func (value V1SoCPercent) String() string {
	negative := value < 0
	abs := int64(value)
	if negative {
		abs = -abs
	}
	whole, fraction := abs/v1SoCScale, abs%v1SoCScale
	text := strconv.FormatInt(whole, 10)
	if fraction != 0 {
		text += "." + strings.TrimRight(fmt.Sprintf("%03d", fraction), "0")
	}
	if negative {
		return "-" + text
	}
	return text
}

func (value V1SoCPercent) MarshalJSON() ([]byte, error) {
	return json.Marshal(json.Number(value.String()))
}

func ParseV1SoCPercent(raw string) (V1SoCPercent, bool) {
	rational, ok := new(big.Rat).SetString(strings.TrimSpace(raw))
	if !ok || rational.Sign() < 0 || rational.Cmp(big.NewRat(100, 1)) > 0 {
		return 0, false
	}
	rational.Mul(rational, big.NewRat(v1SoCScale, 1))
	if !rational.IsInt() || !rational.Num().IsInt64() {
		return 0, false
	}
	return V1SoCPercent(rational.Num().Int64()), true
}

type V1MeterTelemetry struct {
	EnergyWh         *int64
	EnergyObservedAt *time.Time
	SoCPercent       *V1SoCPercent
	SoCObservedAt    *time.Time
}

func (telemetry V1MeterTelemetry) Empty() bool {
	return telemetry.EnergyWh == nil && telemetry.SoCPercent == nil
}

type V1TelemetryUpdateResult struct {
	EnergyAccepted bool
	SoCAccepted    bool
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
	ClaimToken     string
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

// V1BootEvidence is observed hardware metadata. It is never CMS inventory and
// cannot overwrite the canonical mapping identity or expected serial.
type V1BootEvidence struct {
	PathSerial              string
	ChargeBoxSerialNumber   string
	ChargePointSerialNumber string
	ChargePointVendor       string
	ChargePointModel        string
	FirmwareVersion         string
	ObservedAt              time.Time
}

type V1StartMaterialization struct {
	ChargerOCPPIdentity string
	OCPPConnectorNumber int
	IDTag               string
	MeterStartWh        int64
	ActualStartedAt     time.Time
	ObservedAt          time.Time
	OCPPTransactionID   int64
}

type V1Store interface {
	SyncV1Mapping(context.Context, V1MappingInput) (*V1ChargerMapping, bool, error)
	ValidateV1Mapping(context.Context, string, string, string, string, int) error
	ValidateV1ChargerAdmission(context.Context, string, string) error
	CreateV1StartCommand(context.Context, V1StartCommandInput) (*V1RemoteCommand, bool, error)
	CreateV1StopCommand(context.Context, V1StopCommandInput) (*V1RemoteCommand, bool, error)
	GetV1Command(context.Context, string) (*V1RemoteCommand, error)
	GetV1Credential(context.Context, string) (*V1Credential, error)
	MaterializeV1Start(context.Context, V1StartMaterialization) (*V1Transaction, bool, error)
	UpdateV1Meter(context.Context, string, int64, int64, time.Time) (*V1Transaction, error)
	UpdateV1TelemetryForOCPP(context.Context, string, int64, V1MeterTelemetry) (*V1Transaction, V1TelemetryUpdateResult, error)
	RequestV1Stop(context.Context, string, string, string) (*V1Transaction, bool, error)
	CompleteV1Transaction(context.Context, string, int64, string, time.Time, time.Time) (*V1Transaction, error)
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
	ListV1DispatchableStops(context.Context, int) ([]*V1StopWorkflow, error)
	ListV1OverdueTransactions(context.Context, time.Time, int) ([]*V1Transaction, error)
	ClaimV1Facts(context.Context, time.Time, int) ([]V1Fact, error)
	MarkV1FactDelivery(context.Context, string, string, int, bool, bool, string, time.Time) error
	RequeueV1Fact(context.Context, string, string) error
	RecordV1ChargerConnection(context.Context, string, int64, bool, time.Time) error
	RenewCurrentV1ChargerConnection(context.Context, string, int64, time.Time) error
	RecordV1ConnectorStatus(context.Context, V1ConnectorRuntime) error
	RecordV1BootEvidence(context.Context, string, V1BootEvidence) error
	GetV1ChargerRuntime(context.Context, string) (*V1ChargerRuntime, error)
	GetV1ConnectorRuntime(context.Context, string) (*V1ConnectorRuntime, error)
	ResetV1ConnectionRuntime(context.Context) error
}
