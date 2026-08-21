package ocpp16hal

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
)

// ConfigurationReconciliationConfig holds only values HAL can safely request
// after a charger boot. The optional vendor profile is deliberately gated by
// an exact vendor match; standard OCPP keys remain independent from it.
type ConfigurationReconciliationConfig struct {
	MeterValueSampleIntervalSeconds int
	Timeout                         time.Duration
	VendorProfile                   string
	Vendor                          string
}

type configurationClient interface {
	GetConfiguration(context.Context, string, []string) (*core.GetConfigurationConfirmation, error)
	ChangeConfiguration(context.Context, string, string, string) (string, error)
}

type configurationOutcome struct {
	Key     string
	Outcome string
}

func (h *HAL) SetConfigurationReconciliation(cfg ConfigurationReconciliationConfig) {
	if cfg.MeterValueSampleIntervalSeconds > 0 {
		h.configurationMeterSampleIntervalSeconds = cfg.MeterValueSampleIntervalSeconds
	}
	if cfg.Timeout > 0 {
		h.configurationReconcileTimeout = cfg.Timeout
	}
	h.vendorConfigurationProfile = strings.TrimSpace(cfg.VendorProfile)
	h.vendorConfigurationVendor = strings.TrimSpace(cfg.Vendor)
}

func (h *HAL) desiredConfiguration(request *core.BootNotificationRequest) map[string]string {
	desired := map[string]string{
		"HeartbeatInterval":        strconv.Itoa(h.heartbeatIntervalSeconds),
		"MeterValueSampleInterval": strconv.Itoa(h.configurationMeterSampleIntervalSeconds),
	}
	if h.vendorConfigurationProfile == "legacy-remote-only" && request != nil && strings.EqualFold(strings.TrimSpace(request.ChargePointVendor), h.vendorConfigurationVendor) {
		// These are vendor extensions, not standard OCPP 1.6 configuration keys.
		// They are opt-in and tied to the configured physical vendor.
		for key, value := range legacyRemoteOnlyVendorProfile {
			desired[key] = value
		}
	}
	return desired
}

var legacyRemoteOnlyVendorProfile = map[string]string{
	"AuthorizeRemoteTxRequests":  "true",
	"LocalAuthorizeOffline":      "false",
	"LocalPreAuthorize":          "false",
	"AuthorizationCacheEnabled":  "false",
	"AllowOfflineTxForUnknownId": "false",
	"StopTransactionOnInvalidId": "true",
	"ChargePointAuthEnable":      "true",
	"FreevendEnabled":            "false",
}

func (h *HAL) scheduleConfigurationReconciliation(identity string, generation int64, request *core.BootNotificationRequest) {
	desired := h.desiredConfiguration(request)
	if len(desired) == 0 || !h.connectionGenerationCurrent(identity, generation) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), h.configurationReconcileTimeout)
		defer cancel()
		outcomes := reconcileConfiguration(ctx, h, identity, desired, func() bool { return h.connectionGenerationCurrent(identity, generation) })
		for _, outcome := range outcomes {
			h.logger.Info("charger configuration reconciliation", "charge_point_id", identity, "connection_generation", generation, "key", outcome.Key, "outcome", outcome.Outcome)
		}
	}()
}

func (h *HAL) connectionGenerationCurrent(identity string, generation int64) bool {
	current, ok := h.connections.current(identity)
	return ok && int64(current.Generation) == generation
}

func reconcileConfiguration(ctx context.Context, client configurationClient, identity string, desired map[string]string, current func() bool) []configurationOutcome {
	keys := make([]string, 0, len(desired))
	for key := range desired {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if !current() {
		return []configurationOutcome{{Outcome: "superseded"}}
	}
	confirmation, err := client.GetConfiguration(ctx, identity, keys)
	if err != nil || confirmation == nil {
		return []configurationOutcome{{Outcome: "get_failed"}}
	}
	currentValues := make(map[string]core.ConfigurationKey, len(confirmation.ConfigurationKey))
	for _, key := range confirmation.ConfigurationKey {
		currentValues[key.Key] = key
	}
	outcomes := make([]configurationOutcome, 0, len(keys))
	for _, key := range keys {
		if !current() {
			outcomes = append(outcomes, configurationOutcome{Key: key, Outcome: "superseded"})
			break
		}
		observed, known := currentValues[key]
		if !known {
			outcomes = append(outcomes, configurationOutcome{Key: key, Outcome: "unsupported"})
			continue
		}
		if observed.Readonly {
			outcomes = append(outcomes, configurationOutcome{Key: key, Outcome: "readonly"})
			continue
		}
		if observed.Value != nil && *observed.Value == desired[key] {
			outcomes = append(outcomes, configurationOutcome{Key: key, Outcome: "already_current"})
			continue
		}
		status, changeErr := client.ChangeConfiguration(ctx, identity, key, desired[key])
		if changeErr != nil {
			outcomes = append(outcomes, configurationOutcome{Key: key, Outcome: "change_failed"})
			continue
		}
		if status != "Accepted" {
			outcomes = append(outcomes, configurationOutcome{Key: key, Outcome: "rejected"})
			continue
		}
		outcomes = append(outcomes, configurationOutcome{Key: key, Outcome: "changed"})
	}
	return outcomes
}
