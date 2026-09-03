package ocpp16hal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	ocpp16 "github.com/lorenzodonini/ocpp-go/ocpp1.6"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

const defaultHeartbeatIntervalSeconds = 300
const defaultMeterValueSampleIntervalSeconds = 15

type HAL struct {
	cs                                      ocpp16.CentralSystem
	registry                                *state.Registry
	connections                             *connectionTracker
	logger                                  *slog.Logger
	v1Store                                 store.V1Store
	heartbeatIntervalSeconds                int
	configurationMeterSampleIntervalSeconds int
	configurationReconcileTimeout           time.Duration
	vendorConfigurationProfile              string
	vendorConfigurationVendor               string
	runtimeMu                               sync.Mutex
	pendingRuntime                          map[string]pendingRuntimeProjection
	identityMu                              sync.RWMutex
	wiredIdentity                           map[string]string
}

type pendingRuntimeProjection struct {
	identity   string
	generation int64
	online     bool
	observedAt time.Time
}

func New(registry *state.Registry, v1Store store.V1Store, logger *slog.Logger) *HAL {
	h := &HAL{
		cs:                                      ocpp16.NewCentralSystem(nil, nil),
		registry:                                registry,
		connections:                             newConnectionTracker(),
		logger:                                  logger,
		v1Store:                                 v1Store,
		heartbeatIntervalSeconds:                defaultHeartbeatIntervalSeconds,
		configurationMeterSampleIntervalSeconds: defaultMeterValueSampleIntervalSeconds,
		configurationReconcileTimeout:           20 * time.Second,
		pendingRuntime:                          make(map[string]pendingRuntimeProjection),
		wiredIdentity:                           make(map[string]string),
	}

	h.cs.SetNewChargingStationValidationHandler(h.validateIncomingCharger)

	h.cs.SetNewChargePointHandler(func(chargePoint ocpp16.ChargePointConnection) {
		chargePointID := h.canonicalIdentity(chargePoint.ID())
		connKey := connectionKey(chargePoint)

		current, previous := h.connections.register(chargePointID, connKey, fmt.Sprint(chargePoint.RemoteAddr()))

		if previous != nil {
			h.logger.Warn(
				"charge point reconnected; superseding previous connection",
				"charge_point_id", chargePointID,
				"previous_generation", previous.Generation,
				"previous_remote_addr", previous.RemoteAddr,
				"current_generation", current.Generation,
				"current_remote_addr", current.RemoteAddr,
			)
		} else {
			h.logger.Info(
				"charge point connected",
				"charge_point_id", chargePointID,
				"remote_addr", chargePoint.RemoteAddr(),
				"connection_generation", current.Generation,
			)
		}

		h.registry.Touch(chargePointID)
		h.persistRuntimeProjection(context.Background(), pendingRuntimeProjection{identity: chargePointID, generation: int64(current.Generation), online: true, observedAt: current.ConnectedAt})
	})

	h.cs.SetChargePointDisconnectedHandler(func(chargePoint ocpp16.ChargePointConnection) {
		wireIdentity := chargePoint.ID()
		chargePointID := h.canonicalIdentity(wireIdentity)
		connKey := connectionKey(chargePoint)

		isCurrent, current := h.connections.unregisterIfCurrent(chargePointID, connKey)
		if !isCurrent {
			if current != nil {
				h.logger.Info(
					"ignoring stale charge point disconnect",
					"charge_point_id", chargePointID,
					"remote_addr", chargePoint.RemoteAddr(),
					"current_generation", current.Generation,
					"current_remote_addr", current.RemoteAddr,
				)
			} else {
				h.logger.Info(
					"ignoring unknown charge point disconnect",
					"charge_point_id", chargePointID,
					"remote_addr", chargePoint.RemoteAddr(),
				)
			}
			return
		}

		h.logger.Info(
			"charge point disconnected",
			"charge_point_id", chargePointID,
			"remote_addr", chargePoint.RemoteAddr(),
			"connection_generation", current.Generation,
		)

		h.registry.MarkOffline(chargePointID)
		h.persistRuntimeProjection(context.Background(), pendingRuntimeProjection{identity: chargePointID, generation: int64(current.Generation), online: false, observedAt: time.Now().UTC()})
		h.forgetWireIdentity(wireIdentity)
	})

	h.cs.SetCoreHandler(h)
	h.cs.SetFirmwareManagementHandler(h)

	return h
}

// SetHeartbeatIntervalSeconds controls the OCPP interval requested at the next
// BootNotification. A non-positive value retains the safe default.
func (h *HAL) SetHeartbeatIntervalSeconds(seconds int) {
	if seconds > 0 {
		h.heartbeatIntervalSeconds = seconds
	}
}

func (h *HAL) Start(port int, path string) {
	h.logger.Info("starting ocpp-go central system", "port", port, "path", path)
	h.cs.Start(port, path)
}

func (h *HAL) Stop() {
	h.cs.Stop()
}

func (h *HAL) Errors() <-chan error {
	return h.cs.Errors()
}

func (h *HAL) RemoteStartTransaction(ctx context.Context, chargerID string, idTag string, connectorID int) (string, error) {
	chargerID = h.wireIdentityFor(chargerID)
	resultCh := make(chan struct {
		status string
		err    error
	}, 1)

	props := []func(*core.RemoteStartTransactionRequest){}
	if connectorID > 0 {
		connectorCopy := connectorID
		props = append(props, func(req *core.RemoteStartTransactionRequest) {
			req.ConnectorId = &connectorCopy
		})
	}

	err := h.cs.RemoteStartTransaction(
		chargerID,
		func(conf *core.RemoteStartTransactionConfirmation, err error) {
			if err != nil {
				resultCh <- struct {
					status string
					err    error
				}{"", err}
				return
			}

			if conf == nil {
				resultCh <- struct {
					status string
					err    error
				}{"", errors.New("nil RemoteStartTransaction confirmation")}
				return
			}

			resultCh <- struct {
				status string
				err    error
			}{string(conf.Status), nil}
		},
		idTag,
		props...,
	)
	if err != nil {
		return "", err
	}

	select {
	case result := <-resultCh:
		return result.status, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (h *HAL) RemoteStopTransaction(ctx context.Context, chargerID string, transactionID int) (string, error) {
	chargerID = h.wireIdentityFor(chargerID)
	resultCh := make(chan struct {
		status string
		err    error
	}, 1)

	err := h.cs.RemoteStopTransaction(
		chargerID,
		func(conf *core.RemoteStopTransactionConfirmation, err error) {
			if err != nil {
				resultCh <- struct {
					status string
					err    error
				}{"", err}
				return
			}

			if conf == nil {
				resultCh <- struct {
					status string
					err    error
				}{"", errors.New("nil RemoteStopTransaction confirmation")}
				return
			}

			resultCh <- struct {
				status string
				err    error
			}{string(conf.Status), nil}
		},
		transactionID,
	)
	if err != nil {
		return "", err
	}

	select {
	case result := <-resultCh:
		return result.status, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (h *HAL) ChangeAvailability(ctx context.Context, chargerID string, connectorID int, availabilityType string) (string, error) {
	chargerID = h.wireIdentityFor(chargerID)
	var t core.AvailabilityType
	switch strings.ToLower(strings.TrimSpace(availabilityType)) {
	case "operative":
		t = core.AvailabilityTypeOperative
	case "inoperative":
		t = core.AvailabilityTypeInoperative
	default:
		return "", fmt.Errorf("invalid availability type %q", availabilityType)
	}

	resultCh := make(chan struct {
		status string
		err    error
	}, 1)

	err := h.cs.ChangeAvailability(
		chargerID,
		func(conf *core.ChangeAvailabilityConfirmation, err error) {
			if err != nil {
				resultCh <- struct {
					status string
					err    error
				}{"", err}
				return
			}
			if conf == nil {
				resultCh <- struct {
					status string
					err    error
				}{"", errors.New("nil ChangeAvailability confirmation")}
				return
			}
			resultCh <- struct {
				status string
				err    error
			}{string(conf.Status), nil}
		},
		connectorID,
		t,
	)
	if err != nil {
		return "", err
	}

	select {
	case result := <-resultCh:
		return result.status, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (h *HAL) ChangeConfiguration(ctx context.Context, chargerID string, key string, value string) (string, error) {
	chargerID = h.wireIdentityFor(chargerID)
	resultCh := make(chan struct {
		status string
		err    error
	}, 1)

	err := h.cs.ChangeConfiguration(
		chargerID,
		func(conf *core.ChangeConfigurationConfirmation, err error) {
			if err != nil {
				resultCh <- struct {
					status string
					err    error
				}{"", err}
				return
			}
			if conf == nil {
				resultCh <- struct {
					status string
					err    error
				}{"", errors.New("nil ChangeConfiguration confirmation")}
				return
			}
			resultCh <- struct {
				status string
				err    error
			}{string(conf.Status), nil}
		},
		key,
		value,
	)
	if err != nil {
		return "", err
	}

	select {
	case result := <-resultCh:
		return result.status, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (h *HAL) ClearCache(ctx context.Context, chargerID string) (string, error) {
	chargerID = h.wireIdentityFor(chargerID)
	resultCh := make(chan struct {
		status string
		err    error
	}, 1)

	err := h.cs.ClearCache(
		chargerID,
		func(conf *core.ClearCacheConfirmation, err error) {
			if err != nil {
				resultCh <- struct {
					status string
					err    error
				}{"", err}
				return
			}
			if conf == nil {
				resultCh <- struct {
					status string
					err    error
				}{"", errors.New("nil ClearCache confirmation")}
				return
			}
			resultCh <- struct {
				status string
				err    error
			}{string(conf.Status), nil}
		},
	)
	if err != nil {
		return "", err
	}

	select {
	case result := <-resultCh:
		return result.status, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (h *HAL) UnlockConnector(ctx context.Context, chargerID string, connectorID int) (string, error) {
	chargerID = h.wireIdentityFor(chargerID)
	resultCh := make(chan struct {
		status string
		err    error
	}, 1)

	err := h.cs.UnlockConnector(
		chargerID,
		func(conf *core.UnlockConnectorConfirmation, err error) {
			if err != nil {
				resultCh <- struct {
					status string
					err    error
				}{"", err}
				return
			}
			if conf == nil {
				resultCh <- struct {
					status string
					err    error
				}{"", errors.New("nil UnlockConnector confirmation")}
				return
			}
			resultCh <- struct {
				status string
				err    error
			}{string(conf.Status), nil}
		},
		connectorID,
	)
	if err != nil {
		return "", err
	}

	select {
	case result := <-resultCh:
		return result.status, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (h *HAL) Reset(ctx context.Context, chargerID string, resetType string) (string, error) {
	chargerID = h.wireIdentityFor(chargerID)
	var t core.ResetType
	switch strings.ToLower(strings.TrimSpace(resetType)) {
	case "soft":
		t = core.ResetTypeSoft
	case "hard":
		t = core.ResetTypeHard
	default:
		return "", fmt.Errorf("invalid reset type %q", resetType)
	}

	resultCh := make(chan struct {
		status string
		err    error
	}, 1)

	err := h.cs.Reset(
		chargerID,
		func(conf *core.ResetConfirmation, err error) {
			if err != nil {
				resultCh <- struct {
					status string
					err    error
				}{"", err}
				return
			}
			if conf == nil {
				resultCh <- struct {
					status string
					err    error
				}{"", errors.New("nil Reset confirmation")}
				return
			}
			resultCh <- struct {
				status string
				err    error
			}{string(conf.Status), nil}
		},
		t,
	)
	if err != nil {
		return "", err
	}

	select {
	case result := <-resultCh:
		return result.status, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (h *HAL) GetConfiguration(ctx context.Context, chargerID string, keys []string) (*core.GetConfigurationConfirmation, error) {
	chargerID = h.wireIdentityFor(chargerID)
	resultCh := make(chan struct {
		conf *core.GetConfigurationConfirmation
		err  error
	}, 1)

	err := h.cs.GetConfiguration(
		chargerID,
		func(conf *core.GetConfigurationConfirmation, err error) {
			resultCh <- struct {
				conf *core.GetConfigurationConfirmation
				err  error
			}{conf, err}
		},
		keys,
	)
	if err != nil {
		return nil, err
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			return nil, result.err
		}
		if result.conf == nil {
			return nil, errors.New("nil GetConfiguration confirmation")
		}
		return result.conf, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (h *HAL) OnAuthorize(chargePointID string, request *core.AuthorizeRequest) (*core.AuthorizeConfirmation, error) {
	chargePointID = h.canonicalIdentity(chargePointID)
	h.registry.Touch(chargePointID)
	if h.v1Store == nil {
		return core.NewAuthorizationConfirmation(types.NewIdTagInfo(types.AuthorizationStatusInvalid)), nil
	}
	// Authorize carries credential material, so trace it only after the
	// credential can be safely correlated to an existing start lifecycle. The
	// identifier itself never enters diagnostic data.
	credential, credentialErr := h.v1Store.GetV1Credential(context.Background(), request.IdTag)
	accepted := credentialErr == nil && h.v1Store.AuthorizeV1Credential(context.Background(), chargePointID, request.IdTag, time.Now().UTC()) == nil
	if credentialErr == nil {
		h.recordV1ConnectorTrace(chargePointID, credential.OCPPConnectorNumber, store.V1TraceEventInput{Source: "CHARGER", Target: "HAL", Category: "OCPP_CALL", Protocol: "OCPP1.6", Phase: "STARTING", Summary: "Authorize received", OccurredAt: time.Now().UTC(), Data: map[string]any{"action": "Authorize"}})
		outcome := "Invalid"
		if accepted {
			outcome = "Accepted"
		}
		h.recordV1ConnectorTrace(chargePointID, credential.OCPPConnectorNumber, store.V1TraceEventInput{Source: "HAL", Target: "CHARGER", Category: "OCPP_CALL", Protocol: "OCPP1.6", Phase: "STARTING", Summary: "Authorize confirmation: " + outcome, OccurredAt: time.Now().UTC(), Data: map[string]any{"action": "Authorize", "result": outcome}})
	}
	if !accepted {
		return core.NewAuthorizationConfirmation(types.NewIdTagInfo(types.AuthorizationStatusInvalid)), nil
	}
	return core.NewAuthorizationConfirmation(types.NewIdTagInfo(types.AuthorizationStatusAccepted)), nil
}

func (h *HAL) OnBootNotification(chargePointID string, request *core.BootNotificationRequest) (*core.BootNotificationConfirmation, error) {
	chargePointID = h.canonicalIdentity(chargePointID)
	h.registry.Touch(chargePointID)
	if h.v1Store != nil && request != nil {
		evidence := store.V1BootEvidence{ChargeBoxSerialNumber: request.ChargeBoxSerialNumber, ChargePointSerialNumber: request.ChargePointSerialNumber, ChargePointVendor: request.ChargePointVendor, ChargePointModel: request.ChargePointModel, FirmwareVersion: request.FirmwareVersion, ObservedAt: time.Now().UTC()}
		if wireIdentity := h.wireIdentityFor(chargePointID); wireIdentity != chargePointID {
			evidence.PathSerial = wireIdentity
		}
		if err := h.v1Store.RecordV1BootEvidence(context.Background(), chargePointID, evidence); err != nil && !errors.Is(err, store.ErrV1MappingNotFound) {
			h.logger.Warn("failed to persist charger boot identity evidence", "charge_point_id", chargePointID, "serial_present", evidence.PathSerial != "" || evidence.ChargePointSerialNumber != "" || evidence.ChargeBoxSerialNumber != "", "error", err)
		}
	}
	if current, ok := h.connections.current(chargePointID); ok {
		h.scheduleConfigurationReconciliation(chargePointID, int64(current.Generation), request)
	}

	return core.NewBootNotificationConfirmation(
		types.NewDateTime(time.Now().UTC()),
		h.heartbeatIntervalSeconds,
		core.RegistrationStatusAccepted,
	), nil
}

func (h *HAL) OnDataTransfer(chargePointID string, request *core.DataTransferRequest) (*core.DataTransferConfirmation, error) {
	chargePointID = h.canonicalIdentity(chargePointID)
	h.registry.Touch(chargePointID)
	return core.NewDataTransferConfirmation(core.DataTransferStatusAccepted), nil
}

func (h *HAL) OnHeartbeat(chargePointID string, request *core.HeartbeatRequest) (*core.HeartbeatConfirmation, error) {
	chargePointID = h.canonicalIdentity(chargePointID)
	h.registry.Touch(chargePointID)
	if h.v1Store != nil {
		if current, ok := h.connections.current(chargePointID); ok {
			if err := h.v1Store.RenewCurrentV1ChargerConnection(context.Background(), chargePointID, int64(current.Generation), time.Now().UTC()); err != nil && !errors.Is(err, store.ErrV1MappingNotFound) {
				h.logger.Warn("failed to renew v1 charger connection liveness", "charge_point_id", chargePointID, "connection_generation", current.Generation, "error", err)
			}
		}
	}
	return core.NewHeartbeatConfirmation(types.NewDateTime(time.Now().UTC())), nil
}

func (h *HAL) OnStatusNotification(chargePointID string, request *core.StatusNotificationRequest) (*core.StatusNotificationConfirmation, error) {
	chargePointID = h.canonicalIdentity(chargePointID)
	if h.v1Store == nil {
		return nil, errors.New("StatusNotification requires durable v1 storage")
	}
	observedAt := time.Now().UTC()
	err := h.v1Store.RecordV1ConnectorStatus(context.Background(), store.V1ConnectorRuntime{
		ChargerOCPPIdentity: chargePointID, OCPPConnectorNumber: request.ConnectorId, Status: string(request.Status), ErrorCode: string(request.ErrorCode), Info: request.Info, VendorID: request.VendorId, VendorErrorCode: request.VendorErrorCode, ObservedAt: &observedAt,
	})
	if errors.Is(err, store.ErrV1MappingNotFound) || errors.Is(err, store.ErrV1CredentialRejected) {
		// The charger cannot be correlated to a durable v1 connector. No derived
		// local projection is created, but an ordinary OCPP confirmation avoids
		// turning an unsupported observation into a retry storm.
		return core.NewStatusNotificationConfirmation(), nil
	}
	if err != nil {
		h.logger.Error("failed to durably persist v1 connector status", "charge_point_id", chargePointID, "connector_id", request.ConnectorId, "error", err)
		return nil, errors.New("StatusNotification was not durably persisted")
	}
	if traces, ok := h.v1Store.(store.V1TraceStore); ok {
		if trace, traceErr := traces.FindV1TraceForConnector(context.Background(), chargePointID, request.ConnectorId); traceErr == nil {
			phase, phaseErr := h.connectorStatusTracePhase(context.Background(), trace)
			if phaseErr != nil {
				h.logger.Warn("failed to classify diagnostic connector status trace", "trace_id", trace.TraceID, "hal_transaction_id", trace.HALTransactionID, "error", phaseErr)
			} else if err := traces.AppendV1TraceEvent(context.Background(), trace.TraceID, store.V1TraceEventInput{Source: "CHARGER", Target: "HAL", Category: "STATUS", Protocol: "OCPP1.6", Phase: phase, Summary: fmt.Sprintf("Connector status: %s", request.Status), OccurredAt: observedAt, Data: map[string]any{"status": string(request.Status), "connector_id": request.ConnectorId}}); err != nil {
				h.logger.Warn("failed to persist diagnostic connector status trace", "trace_id", trace.TraceID, "error", err)
			}
		}
	}
	h.registry.ApplyStatusNotification(chargePointID, request.ConnectorId, string(request.Status), string(request.ErrorCode))

	return core.NewStatusNotificationConfirmation(), nil
}

// connectorStatusTracePhase classifies only the diagnostic trace presentation.
// It never derives lifecycle truth from an OCPP connector status. A pre-start
// CMS root has no HAL transaction to inspect, so it remains in the pre-
// materialization STARTING phase. A bound trace must be classified from the durable transaction; if
// that transaction cannot be read, no phase is invented and the diagnostic
// observation is skipped without affecting StatusNotification acknowledgement.
func (h *HAL) connectorStatusTracePhase(ctx context.Context, trace *store.V1Trace) (string, error) {
	if trace.HALTransactionID == "" {
		return "STARTING", nil
	}
	transaction, err := h.v1Store.GetV1Transaction(ctx, trace.HALTransactionID)
	if err != nil {
		return "", err
	}
	if transaction.CompletedAt != nil {
		return "POST_STOP", nil
	}
	return "CHARGING", nil
}

func (h *HAL) OnStartTransaction(chargePointID string, request *core.StartTransactionRequest) (*core.StartTransactionConfirmation, error) {
	chargePointID = h.canonicalIdentity(chargePointID)
	h.registry.Touch(chargePointID)
	if h.v1Store == nil || !strings.HasPrefix(request.IdTag, "appv1_") {
		return core.NewStartTransactionConfirmation(types.NewIdTagInfo(types.AuthorizationStatusInvalid), 0), nil
	}
	receivedAt := time.Now().UTC()
	startedAt := receivedAt
	if request.Timestamp != nil && !request.Timestamp.IsZero() {
		startedAt = request.Timestamp.UTC()
	}
	tx, _, err := h.v1Store.MaterializeV1Start(context.Background(), store.V1StartMaterialization{
		ChargerOCPPIdentity: chargePointID,
		OCPPConnectorNumber: request.ConnectorId,
		IDTag:               request.IdTag,
		MeterStartWh:        int64(request.MeterStart),
		ActualStartedAt:     startedAt,
		ObservedAt:          receivedAt,
		OCPPTransactionID:   store.RandomTransactionID(),
	})
	if err != nil {
		if errors.Is(err, store.ErrV1CredentialRejected) {
			return core.NewStartTransactionConfirmation(types.NewIdTagInfo(types.AuthorizationStatusInvalid), 0), nil
		}
		h.logger.Error("failed to materialize v1 start transaction", "charge_point_id", chargePointID, "connector_id", request.ConnectorId, "error", err)
		return core.NewStartTransactionConfirmation(types.NewIdTagInfo(types.AuthorizationStatusBlocked), 0), nil
	}
	if traces, ok := h.v1Store.(store.V1TraceStore); ok {
		trace, traceErr := traces.EnsureV1TraceForTransaction(context.Background(), tx)
		if traceErr == nil {
			if err := traces.BindV1TraceTransaction(context.Background(), trace.TraceID, tx); err != nil {
				h.logger.Warn("failed to bind diagnostic start trace transaction", "trace_id", trace.TraceID, "error", err)
			}
			if err := traces.AppendV1TraceEvent(context.Background(), trace.TraceID, store.V1TraceEventInput{Source: "CHARGER", Target: "HAL", Category: "OCPP_CALL", Protocol: "OCPP1.6", Phase: "STARTING", Summary: "StartTransaction received", OccurredAt: receivedAt, Data: map[string]any{"action": "StartTransaction", "transaction_id": tx.OCPPTransactionID, "connector_id": request.ConnectorId, "meter_wh": request.MeterStart}}); err != nil {
				h.logger.Warn("failed to persist diagnostic start trace event", "trace_id", trace.TraceID, "error", err)
			}
			if err := traces.AppendV1TraceEvent(context.Background(), trace.TraceID, store.V1TraceEventInput{Source: "HAL", Target: "HAL", Category: "LIFECYCLE", Protocol: "POSTGRES", Phase: "STARTING", Summary: "Transaction materialized", OccurredAt: receivedAt, StateAfter: "ACTIVE", Data: map[string]any{"transaction_id": tx.OCPPTransactionID, "connector_id": request.ConnectorId, "meter_wh": request.MeterStart}}); err != nil {
				h.logger.Warn("failed to persist diagnostic start materialization trace", "trace_id", trace.TraceID, "error", err)
			}
		}
	}
	h.registry.ApplyStartTransaction(chargePointID, request.ConnectorId, tx.OCPPTransactionID, float64(request.MeterStart))
	return core.NewStartTransactionConfirmation(types.NewIdTagInfo(types.AuthorizationStatusAccepted), int(tx.OCPPTransactionID)), nil
}

func (h *HAL) OnMeterValues(chargePointID string, request *core.MeterValuesRequest) (*core.MeterValuesConfirmation, error) {
	chargePointID = h.canonicalIdentity(chargePointID)
	h.registry.Touch(chargePointID)
	if h.HandleV1MeterValues(chargePointID, request) == meterPersistenceFailed {
		return nil, errors.New("MeterValues was not durably persisted")
	}
	return core.NewMeterValuesConfirmation(), nil
}

// persistRuntimeProjection keeps physical connection state local even when a
// durable projection write fails. The same exact observation is retried by the
// lifecycle worker; after a crash, startup resets stale durable state to
// UNKNOWN instead of claiming the charger is still fresh or connected.
func (h *HAL) persistRuntimeProjection(ctx context.Context, observation pendingRuntimeProjection) {
	if h.v1Store == nil {
		return
	}
	err := h.v1Store.RecordV1ChargerConnection(ctx, observation.identity, observation.generation, observation.online, observation.observedAt)
	if err == nil || errors.Is(err, store.ErrV1MappingNotFound) || errors.Is(err, store.ErrV1CredentialRejected) {
		h.runtimeMu.Lock()
		delete(h.pendingRuntime, observation.identity)
		h.runtimeMu.Unlock()
		return
	}
	h.runtimeMu.Lock()
	h.pendingRuntime[observation.identity] = observation
	h.runtimeMu.Unlock()
	h.logger.Warn("failed to project physical charger connection durably; retry scheduled", "charge_point_id", observation.identity, "online", observation.online, "connection_generation", observation.generation, "error", err)
}

func (h *HAL) retryRuntimeProjections(ctx context.Context) {
	h.runtimeMu.Lock()
	pending := make([]pendingRuntimeProjection, 0, len(h.pendingRuntime))
	for _, observation := range h.pendingRuntime {
		pending = append(pending, observation)
	}
	h.runtimeMu.Unlock()
	for _, observation := range pending {
		h.persistRuntimeProjection(ctx, observation)
	}
}

func (h *HAL) OnStopTransaction(chargePointID string, request *core.StopTransactionRequest) (*core.StopTransactionConfirmation, error) {
	chargePointID = h.canonicalIdentity(chargePointID)
	h.registry.Touch(chargePointID)
	if !h.HandleV1StopTransaction(chargePointID, request) {
		return nil, errors.New("StopTransaction was not durably persisted")
	}
	return core.NewStopTransactionConfirmation(), nil
}

func ConnectedURL(host string, port int, chargerID string) string {
	return fmt.Sprintf("ws://%s:%d/%s", host, port, chargerID)
}
