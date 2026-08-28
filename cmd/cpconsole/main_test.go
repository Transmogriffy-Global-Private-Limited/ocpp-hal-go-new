package main

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
)

func TestParseStatusIsCaseInsensitive(t *testing.T) {
	got, ok := parseStatus("suspendedevse")
	if !ok || got != core.ChargePointStatusSuspendedEVSE {
		t.Fatalf("parseStatus() = %q, %t", got, ok)
	}
}

func TestParseErrorCodeRejectsUnknownCode(t *testing.T) {
	if _, ok := parseErrorCode("DefinitelyNotOCPP"); ok {
		t.Fatal("parseErrorCode accepted an unknown code")
	}
}

func TestParseStopReason(t *testing.T) {
	got, ok := parseStopReason("evdisconnected")
	if !ok || got != core.ReasonEVDisconnected {
		t.Fatalf("parseStopReason() = %q, %t", got, ok)
	}
}

func TestOCPPMeterWhUsesOneCanonicalRoundedRegister(t *testing.T) {
	// 6 × 15 s at 7.4 kW adds exactly 185 Wh. Start, MeterValues, and Stop
	// must therefore encode the same integer register value.
	value := 100000.0 + 6*15*7.4*1000/3600
	got, err := ocppMeterWh(value)
	if err != nil || got != 100185 {
		t.Fatalf("ocppMeterWh(%f) = %d, %v", value, got, err)
	}
	if _, err := ocppMeterWh(-1); err == nil {
		t.Fatal("negative meter was accepted")
	}
}

func TestParseBoolPolicy(t *testing.T) {
	if value, ok := parseBoolPolicy("ACCEPT", "accept", "reject"); !ok || !value {
		t.Fatalf("parseBoolPolicy accept = %t, %t", value, ok)
	}
	if value, ok := parseBoolPolicy("reject", "accept", "reject"); !ok || value {
		t.Fatalf("parseBoolPolicy reject = %t, %t", value, ok)
	}
}

func TestNormalizeWebSocketURL(t *testing.T) {
	got, err := normalizeWebSocketURL(" wss://ocpp.example.com/base/ ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "wss://ocpp.example.com/base" {
		t.Fatalf("normalizeWebSocketURL() = %q", got)
	}
	if _, err := normalizeWebSocketURL("https://ocpp.example.com"); err == nil {
		t.Fatal("normalizeWebSocketURL accepted an HTTP URL")
	}
}

func TestSelectHeartbeatInterval(t *testing.T) {
	interval, err := selectHeartbeatInterval(300, 0)
	if err != nil || interval != 300 {
		t.Fatalf("server heartbeat interval=%d err=%v", interval, err)
	}
	interval, err = selectHeartbeatInterval(300, 60)
	if err != nil || interval != 60 {
		t.Fatalf("override heartbeat interval=%d err=%v", interval, err)
	}
	if _, err := selectHeartbeatInterval(0, 0); err == nil {
		t.Fatal("zero server heartbeat interval was accepted")
	}
}

func TestStartupOptionsValidation(t *testing.T) {
	valid := startupOptions{connector: 1, meterStartWh: 100, voltage: 230, soc: 35, autoPowerKW: 7.2}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid startup options: %v", err)
	}
	invalidHeartbeat := valid
	invalidHeartbeat.heartbeatInterval = -1
	if err := invalidHeartbeat.validate(); err == nil {
		t.Fatal("negative heartbeat interval was accepted")
	}
	invalidMeter := valid
	invalidMeter.autoMeterInterval = 10
	if err := invalidMeter.validate(); err != nil {
		t.Fatalf("transaction-bound automatic metering was rejected: %v", err)
	}
	invalidPower := valid
	invalidPower.autoMeterInterval, invalidPower.autoPowerKW = 10, 0
	if err := invalidPower.validate(); err == nil {
		t.Fatal("automatic metering with zero power was accepted")
	}
}

func TestHeartbeatSchedulerStops(t *testing.T) {
	sim := newSimulator("CP-TEST", "Model", "Vendor", 1, 100000, 230, 35)
	var calls atomic.Int32
	sim.heartbeatAction = func(<-chan struct{}) error {
		calls.Add(1)
		return nil
	}
	sim.startHeartbeat(5 * time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() == 0 {
		t.Fatal("heartbeat scheduler did not run")
	}
	sim.stopHeartbeat()
	stoppedAt := calls.Load()
	time.Sleep(20 * time.Millisecond)
	if calls.Load() != stoppedAt {
		t.Fatalf("heartbeat scheduler continued after stop: %d -> %d", stoppedAt, calls.Load())
	}
}

func TestCancelledAutomaticMeterCannotSend(t *testing.T) {
	sim := newSimulator("CP-TEST", "Model", "Vendor", 1, 100000, 230, 35)
	cancel := make(chan struct{})
	close(cancel)
	if err := sim.sendAutomaticMeter(cancel, time.Second, 7.2, 1); !errors.Is(err, errWorkerStopped) {
		t.Fatalf("cancelled automatic meter error=%v", err)
	}
}

func TestAutomaticMeterPausesOutsideCharging(t *testing.T) {
	sim := newSimulator("CP-TEST", "Model", "Vendor", 1, 100000, 230, 35)
	sim.mu.Lock()
	sim.transaction = 1
	sim.status = core.ChargePointStatusSuspendedEV
	sim.mu.Unlock()
	if err := sim.sendAutomaticMeter(make(chan struct{}), time.Second, 7.2, 1); !errors.Is(err, errAutomaticMeterPaused) {
		t.Fatalf("suspended automatic meter error=%v", err)
	}
}

func TestAutomaticMeterWorkerStopsCleanly(t *testing.T) {
	sim := newSimulator("CP-TEST", "Model", "Vendor", 1, 100000, 230, 35)
	sim.mu.Lock()
	sim.transaction = 1
	sim.mu.Unlock()
	if err := sim.startAuto(time.Hour, 7.2); err != nil {
		t.Fatal(err)
	}
	sim.stopAutoAndWait()
	sim.mu.RLock()
	active, interval, power := sim.autoCancel != nil, sim.autoInterval, sim.autoPowerKW
	sim.mu.RUnlock()
	if active || interval != 0 || power != 0 {
		t.Fatalf("automatic meter worker state active=%t interval=%s power=%f", active, interval, power)
	}
}

func TestChangeConfigurationReschedulesHeartbeatUnlessOverridden(t *testing.T) {
	sim := newSimulator("CP-TEST", "Model", "Vendor", 1, 100000, 230, 35)
	sim.heartbeatAction = func(<-chan struct{}) error { return nil }
	t.Cleanup(sim.stopHeartbeat)
	if err := sim.configureBootHeartbeat(300); err != nil {
		t.Fatal(err)
	}
	conf, err := sim.OnChangeConfiguration(&core.ChangeConfigurationRequest{Key: "HeartbeatInterval", Value: "60"})
	if err != nil || conf.Status != core.ConfigurationStatusAccepted {
		t.Fatalf("change configuration=%#v err=%v", conf, err)
	}
	sim.mu.RLock()
	effective, configured := sim.heartbeatEffective, sim.configuration["HeartbeatInterval"]
	sim.mu.RUnlock()
	if effective != 60 || configured != "60" {
		t.Fatalf("heartbeat configuration effective=%d stored=%q", effective, configured)
	}

	sim.setHeartbeatOverride(45)
	if err := sim.configureBootHeartbeat(300); err != nil {
		t.Fatal(err)
	}
	conf, err = sim.OnChangeConfiguration(&core.ChangeConfigurationRequest{Key: "HeartbeatInterval", Value: "30"})
	if err != nil || conf.Status != core.ConfigurationStatusRejected {
		t.Fatalf("override change configuration=%#v err=%v", conf, err)
	}
	sim.mu.RLock()
	effective = sim.heartbeatEffective
	sim.mu.RUnlock()
	if effective != 45 {
		t.Fatalf("CLI override was not retained: %d", effective)
	}
}

func TestRunAutomaticSessionUsesNormalOrder(t *testing.T) {
	var calls []string
	err := runAutomaticSession(autoSessionOptions{idTag: "USER001", meterInterval: time.Minute, powerKW: 7.2}, func() error {
		calls = append(calls, "plug")
		return nil
	}, func() error {
		calls = append(calls, "start")
		return nil
	}, func() error {
		calls = append(calls, "meter")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls, ","); got != "plug,start,meter" {
		t.Fatalf("automatic session order=%q", got)
	}

	calls = nil
	err = runAutomaticSession(autoSessionOptions{idTag: "USER001"}, func() error {
		calls = append(calls, "plug")
		return nil
	}, func() error {
		calls = append(calls, "start")
		return errors.New("rejected")
	}, func() error {
		calls = append(calls, "meter")
		return nil
	})
	if err == nil || strings.Join(calls, ",") != "plug,start" {
		t.Fatalf("failed automatic session err=%v calls=%v", err, calls)
	}
}
