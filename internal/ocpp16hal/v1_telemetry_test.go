package ocpp16hal

import (
	"testing"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
)

func TestExtractV1MeterTelemetryKeepsEnergyAndSoCIndependent(t *testing.T) {
	first := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)
	request := &core.MeterValuesRequest{MeterValue: []types.MeterValue{
		{Timestamp: types.NewDateTime(first), SampledValue: []types.SampledValue{{Value: "125", Measurand: types.MeasurandEnergyActiveImportRegister, Unit: types.UnitOfMeasureKWh}, {Value: "banana", Measurand: types.MeasurandSoC, Unit: types.UnitOfMeasurePercent}}},
		{Timestamp: types.NewDateTime(second), SampledValue: []types.SampledValue{{Value: "67.5", Measurand: types.MeasurandSoC, Unit: types.UnitOfMeasurePercent}}},
	}}
	telemetry := extractV1MeterTelemetry(request)
	if telemetry.EnergyWh == nil || *telemetry.EnergyWh != 125000 || telemetry.EnergyObservedAt == nil || !telemetry.EnergyObservedAt.Equal(first) {
		t.Fatalf("energy telemetry=%+v", telemetry)
	}
	if telemetry.SoCPercent == nil || telemetry.SoCPercent.String() != "67.5" || telemetry.SoCObservedAt == nil || !telemetry.SoCObservedAt.Equal(second) {
		t.Fatalf("soc telemetry=%+v", telemetry)
	}
}

func TestExtractV1MeterTelemetryAcceptsSoCOnlyAndRejectsInvalidSoC(t *testing.T) {
	observed := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name, value, unit string
		want              string
	}{
		{name: "zero", value: "0", unit: "Percent", want: "0"},
		{name: "hundred", value: "100", unit: "", want: "100"},
		{name: "negative", value: "-1", unit: "Percent"},
		{name: "over range", value: "100.001", unit: "Percent"},
		{name: "nan", value: "NaN", unit: "Percent"},
		{name: "infinity", value: "Inf", unit: "Percent"},
		{name: "wrong unit", value: "50", unit: "Wh"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := &core.MeterValuesRequest{MeterValue: []types.MeterValue{{Timestamp: types.NewDateTime(observed), SampledValue: []types.SampledValue{{Value: test.value, Measurand: types.MeasurandSoC, Unit: types.UnitOfMeasure(test.unit)}}}}}
			telemetry := extractV1MeterTelemetry(request)
			if test.want == "" && telemetry.SoCPercent != nil {
				t.Fatalf("invalid SoC accepted: %+v", telemetry)
			}
			if test.want != "" && (telemetry.EnergyWh != nil || telemetry.SoCPercent == nil || telemetry.SoCPercent.String() != test.want || !telemetry.SoCObservedAt.Equal(observed)) {
				t.Fatalf("telemetry=%+v", telemetry)
			}
		})
	}
}

func TestExtractV1MeterTelemetryAcceptsValidSoCWhenEnergyIsUnusable(t *testing.T) {
	observed := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	request := &core.MeterValuesRequest{MeterValue: []types.MeterValue{{Timestamp: types.NewDateTime(observed), SampledValue: []types.SampledValue{
		{Value: "not-an-integer-wh", Measurand: types.MeasurandEnergyActiveImportRegister, Unit: types.UnitOfMeasureWh},
		{Value: "42.25", Measurand: types.MeasurandSoC, Unit: types.UnitOfMeasurePercent},
	}}}}
	telemetry := extractV1MeterTelemetry(request)
	if telemetry.EnergyWh != nil || telemetry.SoCPercent == nil || telemetry.SoCPercent.String() != "42.25" || telemetry.SoCObservedAt == nil || !telemetry.SoCObservedAt.Equal(observed) {
		t.Fatalf("telemetry=%+v", telemetry)
	}
}
