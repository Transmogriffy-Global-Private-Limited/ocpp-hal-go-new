package store

import "testing"

func TestSanitizeV1OperationEvidenceDropsUnsafeConfigurationValue(t *testing.T) {
	safe := sanitizeV1TraceData(map[string]any{
		"unique_id": "op-1", "action": "ChangeConfiguration", "message_type": "CALL",
		"payload": map[string]any{"key": "MeterValueSampleInterval", "redacted": true, "value": "never persist"},
	})
	if len(safe) != 0 {
		t.Fatalf("unsafe value reached trace persistence boundary: %#v", safe)
	}

	safe = sanitizeV1TraceData(map[string]any{
		"unique_id": "op-2", "action": "GetConfiguration", "message_type": "CALL",
		"payload": map[string]any{"configuration_keys": []string{"HeartbeatInterval"}},
	})
	payload, ok := safe["payload"].(map[string]any)
	if !ok || len(payload) != 1 {
		t.Fatalf("expected safe operation projection, got %#v", safe)
	}
}

func TestSanitizeV1TriggerMessageFollowOnRejectsUnsafePayload(t *testing.T) {
	safe := sanitizeV1TraceData(map[string]any{
		"follow_on": true, "expected_message": "Heartbeat", "observed_action": "Heartbeat", "charger_ocpp_identity": "CP-1", "id_tag": "must-not-persist",
	})
	if len(safe) != 0 {
		t.Fatalf("unsafe follow-on data reached trace persistence: %#v", safe)
	}
	safe = sanitizeV1TraceData(map[string]any{
		"follow_on": true, "expected_message": "StatusNotification", "observed_action": "StatusNotification", "charger_ocpp_identity": "CP-1", "connector_number": 2,
	})
	if safe["connector_number"] != 2 || len(safe) != 4 {
		t.Fatalf("safe follow-on data=%#v", safe)
	}
}
