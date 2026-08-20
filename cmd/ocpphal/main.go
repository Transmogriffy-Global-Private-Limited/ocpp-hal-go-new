package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/httpapi"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/ocpp16hal"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/v1facts"
)

func main() {
	cfg := config.Load()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))

	v1Store, err := chooseV1Store(context.Background(), cfg)
	if err != nil {
		logger.Error("failed to initialize v1 store", "error", err)
		os.Exit(1)
	}

	registry := state.NewRegistry()
	hal := ocpp16hal.New(registry, v1Store, logger)
	hal.SetHeartbeatIntervalSeconds(cfg.OCPPHeartbeatIntervalSeconds)
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()
	if err := hal.RecoverV1Lifecycle(workerCtx); err != nil {
		logger.Error("failed to recover v1 lifecycle", "error", err)
		os.Exit(1)
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				if err := hal.EnforceV1Deadlines(workerCtx); err != nil {
					logger.Warn("v1 stop recovery pass failed", "error", err)
				}
			}
		}
	}()
	factWorker, err := v1facts.New(cfg, v1Store, logger)
	if err != nil {
		logger.Error("failed to initialize v1 fact delivery", "error", err)
		os.Exit(1)
	}
	if factWorker != nil {
		go factWorker.Start(workerCtx)
	}

	go func() {
		hal.Start(cfg.OCPPListenPort, cfg.OCPPListenPath)
	}()

	go func() {
		for err := range hal.Errors() {
			logger.Error("ocpp-go error", "error", err)
		}
	}()

	api := httpapi.NewServer(cfg, logger, hal, v1Store)

	restServer := &http.Server{
		Addr:              cfg.RESTListenAddr(),
		Handler:           api.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)

	go func() {
		logger.Info("starting REST API", "addr", restServer.Addr)
		errCh <- restServer.ListenAndServe()
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stopCh:
		logger.Info("shutdown signal received", "signal", sig.String())
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("REST server failed", "error", err)
			os.Exit(1)
		}
	}

	hal.Stop()
	cancelWorkers()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := restServer.Shutdown(ctx); err != nil {
		logger.Error("REST shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("shutdown complete")
}

func chooseV1Store(ctx context.Context, cfg config.Config) (*store.PostgresStore, error) {
	if cfg.V1CMSBearerToken == "" {
		return nil, errors.New("HAL_V1_CMS_BEARER_TOKEN is required")
	}
	postgresStore, err := store.NewPostgresStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}
	if err := postgresStore.ResetV1ConnectionRuntime(ctx); err != nil {
		return nil, fmt.Errorf("reset persisted v1 connection runtime: %w", err)
	}
	return postgresStore, nil
}
