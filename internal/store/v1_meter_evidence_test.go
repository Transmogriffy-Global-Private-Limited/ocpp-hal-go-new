package store

import (
	"errors"
	"testing"
)

func TestClassifyV1MeterEvidence(t *testing.T) {
	latest := int64(100185)
	tests := []struct {
		name       string
		start      int64
		latest     *int64
		raw        int64
		wantClass  v1MeterEvidenceClass
		wantValue  int64
		wantAdjust int64
		wantErr    bool
	}{
		{name: "first exact", start: 100000, raw: 100000, wantClass: v1MeterEvidenceExact, wantValue: 100000},
		{name: "same reading exact", start: 100000, latest: &latest, raw: 100185, wantClass: v1MeterEvidenceExact, wantValue: 100185},
		{name: "forward", start: 100000, latest: &latest, raw: 100186, wantClass: v1MeterEvidenceForward, wantValue: 100186},
		{name: "one Wh rounding rollback normalizes", start: 100000, latest: &latest, raw: 100184, wantClass: v1MeterEvidenceQuantizationNormalized, wantValue: 100185, wantAdjust: 1},
		{name: "larger rollback rejects", start: 100000, latest: &latest, raw: 100183, wantClass: v1MeterEvidenceContradictory, wantErr: true},
		{name: "below start rejects", start: 100000, raw: 99999, wantClass: v1MeterEvidenceContradictory, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := classifyV1MeterEvidence(tt.start, tt.latest, tt.raw)
			if tt.wantErr {
				if !errors.Is(err, ErrV1InvalidEvidence) {
					t.Fatalf("error = %v, want invalid evidence", err)
				}
				return
			}
			if err != nil || got.Class != tt.wantClass || got.EffectiveWh != tt.wantValue || got.AdjustmentWh != tt.wantAdjust {
				t.Fatalf("classification = %#v, err = %v", got, err)
			}
		})
	}
}
