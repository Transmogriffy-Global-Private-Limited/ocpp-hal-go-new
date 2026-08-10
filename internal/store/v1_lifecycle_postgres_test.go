package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCanonicalV1JSONStableForFactValues(t *testing.T) {
	first, err := canonicalV1JSON(map[string]any{"z": int64(7), "a": map[string]any{"observed_at": time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC), "value_wh": int64(42)}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := canonicalV1JSON(map[string]any{"a": map[string]any{"value_wh": int64(42), "observed_at": time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}, "z": int64(7)})
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || string(first) != `{"a":{"observed_at":"2026-08-10T12:00:00Z","value_wh":42},"z":7}` {
		t.Fatalf("canonical JSON=%s", first)
	}
}

func TestCanonicalV1JSONRFC8785Vector(t *testing.T) {
	input := json.RawMessage(`{"numbers":[333333333.33333329,1E30,4.50,2e-3,0.000000000000000000000000001],"string":"\u20ac$\u000F\u000aA'\u0042\u0022\u005c\\\"\/","literals":[null,true,false]}`)
	got, err := canonicalV1JSON(input)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"literals":[null,true,false],"numbers":[333333333.3333333,1e+30,4.5,0.002,1e-27],"string":"€$\u000f\nA'B\"\\\\\"/"}`
	if string(got) != want {
		t.Fatalf("canonical JSON=%s, want %s", got, want)
	}
}

func TestV1FactPayloadsConformToApprovedFieldNames(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	transaction := &V1Transaction{
		HALTransactionID: "hal-transaction", CMSCommandID: "start-command", CMSStartIntentID: "start-intent", CPOID: "cpo",
		CMSChargerID: "charger", CMSConnectorID: "connector", ChargerOCPPIdentity: "CP-1", OCPPConnectorNumber: 1,
		IDTag: "appv1_test", OCPPTransactionID: 73, ActualStartedAt: now, MeterStartWh: 12000,
		LatestMeterWh: int64ptr(12345), MeterObservedAt: timePtr(now.Add(time.Second)), MeterSequence: 1,
		MeterStopWh: int64ptr(13000), CompletedAt: timePtr(now.Add(2 * time.Second)), OCPPStopReason: "Local",
	}
	started := decodedFactPayload(t, v1StartedFact(transaction, "hal-start-command"))
	if _, ok := started["actual_started_at"]; ok || started["started_at"] == nil {
		t.Fatalf("started payload=%#v", started)
	}
	meter := decodedFactPayload(t, v1MeterFact(transaction))
	for _, field := range []string{"meter_sequence", "meter_value_wh", "consumed_wh", "meter_observed_at"} {
		if meter[field] == nil {
			t.Fatalf("meter payload missing %s: %#v", field, meter)
		}
	}
	command := decodedFactPayload(t, v1CommandFact(&V1RemoteCommand{HALCommandID: "hal-command", CMSCommandID: "cms-command", Kind: "START", State: "AMBIGUOUS", ChargerOCPPIdentity: "CP-1", OCPPConnectorNumber: 1, DeliveryAttempts: 1, LastErrorCategory: "delivery", LastErrorDetail: "response lost", UpdatedAt: now}))
	if command["last_error_category"] != "delivery" || command["last_error_detail"] != "response lost" {
		t.Fatalf("command payload=%#v", command)
	}
	completed := decodedFactPayload(t, v1CompletedFact(transaction, &V1RemoteCommand{HALCommandID: "hal-stop-command", CMSCommandID: "cms-stop-command"}))
	if completed["hal_command_id"] != "hal-stop-command" || completed["cms_command_id"] != "cms-stop-command" {
		t.Fatalf("completed payload=%#v", completed)
	}
	natural := decodedFactPayload(t, v1CompletedFact(transaction, nil))
	if natural["hal_command_id"] != nil || natural["cms_command_id"] != nil {
		t.Fatalf("natural completion payload=%#v", natural)
	}
}

func decodedFactPayload(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func int64ptr(value int64) *int64 { return &value }

func timePtr(value time.Time) *time.Time { return &value }
