package store

import (
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
