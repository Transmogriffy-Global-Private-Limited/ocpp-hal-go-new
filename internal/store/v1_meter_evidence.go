package store

import "errors"

// v1MeterQuantizationToleranceWh is intentionally protocol-local and fixed.
// It covers one integer-register rounding/truncation disagreement, not a
// general permission to accept regressive charger evidence.
const v1MeterQuantizationToleranceWh int64 = 1

type v1MeterEvidenceClass string

const (
	v1MeterEvidenceExact                  v1MeterEvidenceClass = "EXACT"
	v1MeterEvidenceForward                v1MeterEvidenceClass = "FORWARD"
	v1MeterEvidenceQuantizationNormalized v1MeterEvidenceClass = "QUANTIZATION_NORMALIZED"
	v1MeterEvidenceContradictory          v1MeterEvidenceClass = "CONTRADICTORY"
)

type v1MeterEvidence struct {
	Class        v1MeterEvidenceClass
	EffectiveWh  int64
	AdjustmentWh int64
}

func classifyV1MeterEvidence(meterStartWh int64, latestMeterWh *int64, rawMeterWh int64) (v1MeterEvidence, error) {
	if rawMeterWh < meterStartWh {
		return v1MeterEvidence{Class: v1MeterEvidenceContradictory}, ErrV1InvalidEvidence
	}
	if latestMeterWh == nil || rawMeterWh == *latestMeterWh {
		return v1MeterEvidence{Class: v1MeterEvidenceExact, EffectiveWh: rawMeterWh}, nil
	}
	if rawMeterWh > *latestMeterWh {
		return v1MeterEvidence{Class: v1MeterEvidenceForward, EffectiveWh: rawMeterWh}, nil
	}
	adjustment := *latestMeterWh - rawMeterWh
	if adjustment <= v1MeterQuantizationToleranceWh {
		return v1MeterEvidence{Class: v1MeterEvidenceQuantizationNormalized, EffectiveWh: *latestMeterWh, AdjustmentWh: adjustment}, nil
	}
	return v1MeterEvidence{Class: v1MeterEvidenceContradictory}, errors.Join(ErrV1InvalidEvidence, errors.New("meter rollback exceeds quantization tolerance"))
}
