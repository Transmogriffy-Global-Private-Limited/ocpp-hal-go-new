package ocpp16hal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	ocpp16 "github.com/lorenzodonini/ocpp-go/ocpp1.6"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

const heartbeatIntervalSeconds = 900

type HAL struct {
	cs          ocpp16.CentralSystem
	registry    *state.Registry
	connections *connectionTracker
	logger      *slog.Logger
	v1Store     store.V1Store
}

func New(registry *state.Registry, v1Store store.V1Store, logger *slog.Logger) *HAL {
	h := &HAL{
		cs:          ocpp16.NewCentralSystem(nil, nil),
		registry:    registry,
		connections: newConnectionTracker(),
		logger:      logger,
		v1Store:     v1Store,
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
		if h.v1Store != nil {
			if err := h.v1Store.RecordV1ChargerConnection(context.Background(), chargePointID, int64(current.Generation), true, current.ConnectedAt); err != nil && !errors.Is(err, store.ErrV1MappingNotFound) {
				h.logger.Warn("failed to persist v1 charger connection", "charge_point_id", chargePointID, "error", err)
			}
		}
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
		if h.v1Store != nil {
			if err := h.v1Store.RecordV1ChargerConnection(context.Background(), chargePointID, int64(current.Generation), false, time.Now().UTC()); err != nil && !errors.Is(err, store.ErrV1MappingNotFound) {
				h.logger.Warn("failed to persist v1 charger disconnect", "charge_point_id", chargePointID, "error", err)
			}
		}
	})

	h.cs.SetCoreHandler(h)
	h.cs.SetFirmwareManagementHandler(h)

	return h
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
		heartbeatIntervalSeconds,
		core.RegistrationStatusAccepted,
	), nil
}

func (h *HAL) OnDataTransfer(chargePointID string, request *core.DataTransferRequest) (*core.DataTransferConfirmation, error) {
	h.registry.Touch(chargePointID)
	return core.NewDataTransferConfirmation(core.DataTransferStatusAccepted), nil
}

func (h *HAL) OnHeartbeat(chargePointID string, request *core.HeartbeatRequest) (*core.HeartbeatConfirmation, error) {
	h.registry.Touch(chargePointID)
	return core.NewHeartbeatConfirmation(types.NewDateTime(time.Now().UTC())), nil
}

func (h *HAL) OnStatusNotification(chargePointID string, request *core.StatusNotificationRequest) (*core.StatusNotificationConfirmation, error) {
	h.registry.ApplyStatusNotification(
		chargePointID,
		request.ConnectorId,
		string(request.Status),
		string(request.ErrorCode),
	)
	if h.v1Store != nil {
		observedAt := time.Now().UTC()
		if request.Timestamp != nil && !request.Timestamp.IsZero() {
			observedAt = request.Timestamp.UTC()
		}
		err := h.v1Store.RecordV1ConnectorStatus(context.Background(), store.V1ConnectorRuntime{
			ChargerOCPPIdentity: chargePointID,
			OCPPConnectorNumber: request.ConnectorId,
			Status:              string(request.Status),
			ErrorCode:           string(request.ErrorCode),
			Info:                request.Info,
			VendorID:            request.VendorId,
			VendorErrorCode:     request.VendorErrorCode,
			ObservedAt:          &observedAt,
		})
		if err != nil && !errors.Is(err, store.ErrV1MappingNotFound) {
			h.logger.Warn("failed to persist v1 connector status", "charge_point_id", chargePointID, "connector_id", request.ConnectorId, "error", err)
		}
	}

	return core.NewStatusNotificationConfirmation(), nil
}

func (h *HAL) OnStartTransaction(chargePointID string, request *core.StartTransactionRequest) (*core.StartTransactionConfirmation, error) {
	h.registry.Touch(chargePointID)
	if h.v1Store == nil || !strings.HasPrefix(request.IdTag, "appv1_") {
		return core.NewStartTransactionConfirmation(types.NewIdTagInfo(types.AuthorizationStatusInvalid), 0), nil
	}
	startedAt := time.Now().UTC()
	if request.Timestamp != nil && !request.Timestamp.IsZero() {
		startedAt = request.Timestamp.UTC()
	}
	tx, _, err := h.v1Store.MaterializeV1Start(context.Background(), store.V1StartMaterialization{
		ChargerOCPPIdentity: chargePointID,
		OCPPConnectorNumber: request.ConnectorId,
		IDTag:               request.IdTag,
		MeterStartWh:        int64(request.MeterStart),
		ActualStartedAt:     startedAt,
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
	_ = h.HandleV1MeterValues(chargePointID, request)
	return core.NewMeterValuesConfirmation(), nil
}

func (h *HAL) OnStopTransaction(chargePointID string, request *core.StopTransactionRequest) (*core.StopTransactionConfirmation, error) {
	h.registry.Touch(chargePointID)
	_ = h.HandleV1StopTransaction(chargePointID, request)
	return core.NewStopTransactionConfirmation(), nil
}

func ConnectedURL(host string, port int, chargerID string) string {
	return fmt.Sprintf("ws://%s:%d/%s", host, port, chargerID)
}
