package ocpp16hal

import (
	"context"
	"testing"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
)

type fakeConfigurationClient struct {
	confirmation *core.GetConfigurationConfirmation
	changes      []string
}

func (c *fakeConfigurationClient) GetConfiguration(context.Context, string, []string) (*core.GetConfigurationConfirmation, error) {
	return c.confirmation, nil
}

func (c *fakeConfigurationClient) ChangeConfiguration(_ context.Context, _ string, key, value string) (string, error) {
	c.changes = append(c.changes, key+"="+value)
	return "Accepted", nil
}

func TestReconcileConfigurationContinuesPastUnsupportedReadonlyAndRejectedKeys(t *testing.T) {
	current := "300"
	readonly := "0"
	client := &fakeConfigurationClient{confirmation: &core.GetConfigurationConfirmation{ConfigurationKey: []core.ConfigurationKey{
		{Key: "HeartbeatInterval", Value: &current},
		{Key: "MeterValueSampleInterval", Value: &readonly, Readonly: true},
		{Key: "AuthorizeRemoteTxRequests", Value: nil},
	}}}
	outcomes := reconcileConfiguration(context.Background(), client, "CP-1", map[string]string{
		"HeartbeatInterval": "300", "MeterValueSampleInterval": "15", "Unknown": "value", "AuthorizeRemoteTxRequests": "true",
	}, func() bool { return true })
	if len(client.changes) != 1 || client.changes[0] != "AuthorizeRemoteTxRequests=true" {
		t.Fatalf("changes=%v", client.changes)
	}
	got := map[string]string{}
	for _, outcome := range outcomes {
		got[outcome.Key] = outcome.Outcome
	}
	if got["HeartbeatInterval"] != "already_current" || got["MeterValueSampleInterval"] != "readonly" || got["Unknown"] != "unsupported" || got["AuthorizeRemoteTxRequests"] != "changed" {
		t.Fatalf("outcomes=%v", outcomes)
	}
}

func TestReconcileConfigurationStopsAtGenerationFence(t *testing.T) {
	client := &fakeConfigurationClient{confirmation: &core.GetConfigurationConfirmation{}}
	outcomes := reconcileConfiguration(context.Background(), client, "CP-1", map[string]string{"HeartbeatInterval": "300"}, func() bool { return false })
	if len(outcomes) != 1 || outcomes[0].Outcome != "superseded" || len(client.changes) != 0 {
		t.Fatalf("outcomes=%v changes=%v", outcomes, client.changes)
	}
}

func TestVendorConfigurationProfileIsExplicitlyGated(t *testing.T) {
	h := &HAL{heartbeatIntervalSeconds: 300, configurationMeterSampleIntervalSeconds: 15, vendorConfigurationProfile: "legacy-remote-only", vendorConfigurationVendor: "Known Vendor"}
	if desired := h.desiredConfiguration(core.NewBootNotificationRequest("M", "Other Vendor")); len(desired) != 2 {
		t.Fatalf("unexpected vendor extensions: %v", desired)
	}
	if desired := h.desiredConfiguration(core.NewBootNotificationRequest("M", "Known Vendor")); desired["FreevendEnabled"] != "false" || desired["HeartbeatInterval"] != "300" {
		t.Fatalf("profile=%v", desired)
	}
}
