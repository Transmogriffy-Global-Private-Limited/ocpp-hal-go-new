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

type HAL struct {
	cs                       ocpp16.CentralSystem
	registry                 *state.Registry
	connections              *connectionTracker
	logger                   *slog.Logger
	v1Store                  store.V1Store
	heartbeatIntervalSeconds int
	runtimeMu                sync.Mutex
	pendingRuntime           map[string]pendingRuntimeProjection
}

type pendingRuntimeProjection struct {
	identity   string
	generation int64
	online     bool
	observedAt time.Time
}

func New(registry *state.Registry, v1Store store.V1Store, logger *slog.Logger) *HAL {
	h := &HAL{
		cs:                       ocpp16.NewCentralSystem(nil, nil),
		registry:                 registry,
		connections:              newConnectionTracker(),
		logger:                   logger,
		v1Store:                  v1Store,
		heartbeatIntervalSeconds: defaultHeartbeatIntervalSeconds,
		pendingRuntime:           make(map[string]pendingRuntimeProjection),
	}

	h.cs.SetNewChargingStationValidationHandler(h.validateIncomingCharger)

	h.cs.SetNewChargePointHandler(func(chargePoint ocpp16.ChargePointConnection) {
		chargePointID := chargePoint.ID()
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
		chargePointID := chargePoint.ID()
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
	h.registry.Touch(chargePointID)
	if h.v1Store == nil || h.v1Store.AuthorizeV1Credential(context.Background(), chargePointID, request.IdTag, time.Now().UTC()) != nil {
		return core.NewAuthorizationConfirmation(types.NewIdTagInfo(types.AuthorizationStatusInvalid)), nil
	}
	return core.NewAuthorizationConfirmation(types.NewIdTagInfo(types.AuthorizationStatusAccepted)), nil
}

func (h *HAL) OnBootNotification(chargePointID string, request *core.BootNotificationRequest) (*core.BootNotificationConfirmation, error) {
	h.registry.Touch(chargePointID)

	return core.NewBootNotificationConfirmation(
		types.NewDateTime(time.Now().UTC()),
		h.heartbeatIntervalSeconds,
		core.RegistrationStatusAccepted,
	), nil
}

func (h *HAL) OnDataTransfer(chargePointID string, request *core.DataTransferRequest) (*core.DataTransferConfirmation, error) {
	h.registry.Touch(chargePointID)
	return core.NewDataTransferConfirmation(core.DataTransferStatusAccepted), nil
}

func (h *HAL) OnHeartbeat(chargePointID string, request *core.HeartbeatRequest) (*core.HeartbeatConfirmation, error) {
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
	h.registry.ApplyStatusNotification(chargePointID, request.ConnectorId, string(request.Status), string(request.ErrorCode))

	return core.NewStatusNotificationConfirmation(), nil
}

func (h *HAL) OnStartTransaction(chargePointID string, request *core.StartTransactionRequest) (*core.StartTransactionConfirmation, error) {
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
	h.registry.ApplyStartTransaction(chargePointID, request.ConnectorId, tx.OCPPTransactionID, float64(request.MeterStart))
	return core.NewStartTransactionConfirmation(types.NewIdTagInfo(types.AuthorizationStatusAccepted), int(tx.OCPPTransactionID)), nil
}

func (h *HAL) OnMeterValues(chargePointID string, request *core.MeterValuesRequest) (*core.MeterValuesConfirmation, error) {
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
	h.registry.Touch(chargePointID)
	if !h.HandleV1StopTransaction(chargePointID, request) {
		return nil, errors.New("StopTransaction was not durably persisted")
	}
	return core.NewStopTransactionConfirmation(), nil
}

func ConnectedURL(host string, port int, chargerID string) string {
	return fmt.Sprintf("ws://%s:%d/%s", host, port, chargerID)
}
