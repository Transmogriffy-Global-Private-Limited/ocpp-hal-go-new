package main

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
)

func acceptedInfo() *types.IdTagInfo { return types.NewIdTagInfo(types.AuthorizationStatusAccepted) }

func testSimulator(count int) *simulator {
	s := newSimulator("CP-TEST", "Model", "Vendor", count, 1, 100000, 230, 35)
	s.autoRemote = false
	s.statusAction = func(int, core.ChargePointErrorCode, core.ChargePointStatus) error { return nil }
	s.authorizeAction = func(string) (*core.AuthorizeConfirmation, error) {
		return core.NewAuthorizationConfirmation(acceptedInfo()), nil
	}
	s.startAction = func(_ int, _ string, _ int) (*core.StartTransactionConfirmation, error) {
		return core.NewStartTransactionConfirmation(acceptedInfo(), 901), nil
	}
	s.meterAction = func(int, []types.MeterValue, int) error { return nil }
	s.stopAction = func(_ int, _ int, _ string, _ core.Reason) (*core.StopTransactionConfirmation, error) {
		return core.NewStopTransactionConfirmation(), nil
	}
	return s
}

func TestStartupOptionsAndConnectorConstruction(t *testing.T) {
	opts := startupOptions{connectors: 2, connector: 1, meterStartWh: 100, voltage: 230, soc: 35, autoPowerKW: 7.2}
	if err := opts.validate(); err != nil {
		t.Fatal(err)
	}
	bad := opts
	bad.connector = 3
	if err := bad.validate(); err == nil {
		t.Fatal("out-of-range selected connector accepted")
	}
	s := testSimulator(2)
	if got := s.connectorIDs(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("configured connectors = %v", got)
	}
	if s.cp == nil {
		t.Fatal("simulator has no charge-point transport")
	}
}

func TestBootOnceAndStartupStatusForEachConnector(t *testing.T) {
	s := testSimulator(2)
	var bootCount int
	var statuses []int
	s.bootAction = func() (*core.BootNotificationConfirmation, error) {
		bootCount++
		return core.NewBootNotificationConfirmation(types.Now(), 300, core.RegistrationStatusAccepted), nil
	}
	s.statusAction = func(id int, _ core.ChargePointErrorCode, status core.ChargePointStatus) error {
		if status != core.ChargePointStatusAvailable {
			t.Fatalf("startup status=%s", status)
		}
		statuses = append(statuses, id)
		return nil
	}
	if err := s.boot(); err != nil {
		t.Fatal(err)
	}
	for _, id := range s.connectorIDs() {
		c, _ := s.connector(id)
		if err := s.setStatus(c, core.ChargePointStatusAvailable, core.NoError); err != nil {
			t.Fatal(err)
		}
	}
	s.stopHeartbeat()
	if bootCount != 1 || strings.Join([]string{string(rune(statuses[0] + '0')), string(rune(statuses[1] + '0'))}, ",") != "1,2" {
		t.Fatalf("boot=%d statuses=%v", bootCount, statuses)
	}
}

func TestRemoteStartRoutesExplicitAndOmittedToEligibleConnector(t *testing.T) {
	s := testSimulator(2)
	c1, _ := s.connector(1)
	c2, _ := s.connector(2)
	c1.mu.Lock()
	c1.status = core.ChargePointStatusCharging
	c1.transaction = 111
	c1.mu.Unlock()
	c2.mu.Lock()
	c2.status = core.ChargePointStatusAvailable
	c2.mu.Unlock()
	requested := 2
	conf, err := s.OnRemoteStartTransaction(&core.RemoteStartTransactionRequest{IdTag: "TAG2", ConnectorId: &requested})
	if err != nil || conf.Status != types.RemoteStartStopStatusAccepted {
		t.Fatalf("explicit remote start=%#v err=%v", conf, err)
	}
	c2.mu.RLock()
	pending := c2.remoteStart != nil
	c2.mu.RUnlock()
	if !pending {
		t.Fatal("explicit connector did not retain pending start")
	}
	activeConnector := 1
	conf, err = s.OnRemoteStartTransaction(&core.RemoteStartTransactionRequest{IdTag: "ACTIVE", ConnectorId: &activeConnector})
	if err != nil || conf.Status != types.RemoteStartStopStatusRejected {
		t.Fatalf("active connector remote start=%#v err=%v", conf, err)
	}
	c2.mu.Lock()
	c2.remoteStart = nil
	c2.mu.Unlock()
	conf, err = s.OnRemoteStartTransaction(&core.RemoteStartTransactionRequest{IdTag: "FALLBACK"})
	if err != nil || conf.Status != types.RemoteStartStopStatusAccepted {
		t.Fatalf("fallback remote start=%#v err=%v", conf, err)
	}
	c2.mu.RLock()
	pending = c2.remoteStart != nil
	c2.mu.RUnlock()
	if !pending {
		t.Fatal("omitted connector did not choose lowest eligible connector")
	}
}

func TestRemoteStartRejectsUnknownAndImpossibleConnector(t *testing.T) {
	s := testSimulator(2)
	unknown := 3
	conf, _ := s.OnRemoteStartTransaction(&core.RemoteStartTransactionRequest{IdTag: "TAG", ConnectorId: &unknown})
	if conf.Status != types.RemoteStartStopStatusRejected {
		t.Fatal("unknown connector was accepted")
	}
	for _, id := range s.connectorIDs() {
		c, _ := s.connector(id)
		c.mu.Lock()
		c.status = core.ChargePointStatusFaulted
		c.mu.Unlock()
	}
	conf, _ = s.OnRemoteStartTransaction(&core.RemoteStartTransactionRequest{IdTag: "TAG"})
	if conf.Status != types.RemoteStartStopStatusRejected {
		t.Fatal("impossible connector selection was accepted")
	}
}

func TestRemoteStopTargetsExactTransactionAndCompletesOnlyOwner(t *testing.T) {
	s := testSimulator(2)
	c1, _ := s.connector(1)
	c2, _ := s.connector(2)
	c1.mu.Lock()
	c1.transaction, c1.idTag, c1.status = 101, "ONE", core.ChargePointStatusCharging
	c1.mu.Unlock()
	c2.mu.Lock()
	c2.transaction, c2.idTag, c2.status = 202, "TWO", core.ChargePointStatusCharging
	c2.mu.Unlock()
	var stopTx int
	s.stopAction = func(_ int, tx int, _ string, _ core.Reason) (*core.StopTransactionConfirmation, error) {
		stopTx = tx
		return core.NewStopTransactionConfirmation(), nil
	}
	conf, err := s.OnRemoteStopTransaction(&core.RemoteStopTransactionRequest{TransactionId: 202})
	if err != nil || conf.Status != types.RemoteStartStopStatusAccepted {
		t.Fatalf("remote stop=%#v err=%v", conf, err)
	}
	if err := s.executePendingRemoteStop(c2); err != nil {
		t.Fatal(err)
	}
	if stopTx != 202 {
		t.Fatalf("stopped transaction=%d", stopTx)
	}
	c1.mu.RLock()
	tx1, status1 := c1.transaction, c1.status
	c1.mu.RUnlock()
	c2.mu.RLock()
	tx2, status2 := c2.transaction, c2.status
	c2.mu.RUnlock()
	if tx1 != 101 || status1 != core.ChargePointStatusCharging || tx2 != 0 || status2 != core.ChargePointStatusAvailable {
		t.Fatalf("connector isolation owner=%d/%s other=%d/%s", tx2, status2, tx1, status1)
	}
	if err := s.executePendingRemoteStop(c2); err == nil {
		t.Fatal("duplicate pending remote stop executed")
	}
}

func TestMeteringCarriesConnectorAndTransactionIndependently(t *testing.T) {
	s := testSimulator(2)
	c1, _ := s.connector(1)
	c2, _ := s.connector(2)
	c1.mu.Lock()
	c1.transaction, c1.status = 11, core.ChargePointStatusCharging
	c1.mu.Unlock()
	c2.mu.Lock()
	c2.transaction, c2.status = 22, core.ChargePointStatusCharging
	c2.mu.Unlock()
	type call struct{ connector, tx int }
	var calls []call
	s.meterAction = func(id int, _ []types.MeterValue, tx int) error { calls = append(calls, call{id, tx}); return nil }
	if err := s.sendMeter(c1, time.Minute, 7.2); err != nil {
		t.Fatal(err)
	}
	if err := s.sendMeter(c2, 30*time.Second, 3.6); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != (call{1, 11}) || calls[1] != (call{2, 22}) {
		t.Fatalf("meter calls=%v", calls)
	}
	c1.mu.RLock()
	m1 := c1.meterWh
	c1.mu.RUnlock()
	c2.mu.RLock()
	m2 := c2.meterWh
	c2.mu.RUnlock()
	if m1 == m2 {
		t.Fatalf("meter registers were not independent: %f %f", m1, m2)
	}
}

func TestAutomaticWorkersAreIndependent(t *testing.T) {
	s := testSimulator(2)
	c1, _ := s.connector(1)
	c2, _ := s.connector(2)
	for _, c := range []*connectorState{c1, c2} {
		c.mu.Lock()
		c.transaction, c.status = c.id, core.ChargePointStatusCharging
		c.mu.Unlock()
	}
	var calls atomic.Int32
	s.meterAction = func(int, []types.MeterValue, int) error { calls.Add(1); return nil }
	if err := s.startAuto(c1, 5*time.Millisecond, 7.2); err != nil {
		t.Fatal(err)
	}
	if err := s.startAuto(c2, 5*time.Millisecond, 7.2); err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	c1.stopAutoAndWait()
	before := calls.Load()
	time.Sleep(20 * time.Millisecond)
	c2.mu.RLock()
	running := c2.autoCancel != nil
	c2.mu.RUnlock()
	c2.stopAutoAndWait()
	if !running || calls.Load() <= before {
		t.Fatalf("connector 2 worker did not continue independently: before=%d after=%d running=%t", before, calls.Load(), running)
	}
}

func TestConfiguredAutomaticMeterBindsToEachStartedTransaction(t *testing.T) {
	s := testSimulator(1)
	s.setAutomaticMeterConfig(time.Hour, 7.2)
	c, _ := s.connector(1)
	c.mu.Lock()
	c.status = core.ChargePointStatusAvailable
	c.mu.Unlock()
	if err := s.startTransaction(c, "TAG", false); err != nil {
		t.Fatal(err)
	}
	c.mu.RLock()
	active, transaction := c.autoCancel != nil, c.autoTransaction
	c.mu.RUnlock()
	c.stopAutoAndWait()
	if !active || transaction != 901 {
		t.Fatalf("automatic meter binding active=%t transaction=%d", active, transaction)
	}
	withoutStartupTag := startupOptions{connectors: 1, connector: 1, meterStartWh: 100, voltage: 230, soc: 35, autoPowerKW: 7.2, autoMeterInterval: 10}
	if err := withoutStartupTag.validate(); err != nil {
		t.Fatalf("transaction-bound automatic metering was rejected: %v", err)
	}
}

func TestChangeAvailabilityZeroTargetsAllAndSelectedCommandsRoute(t *testing.T) {
	s := testSimulator(2)
	var mu sync.Mutex
	var ids []int
	s.statusAction = func(id int, _ core.ChargePointErrorCode, _ core.ChargePointStatus) error {
		mu.Lock()
		ids = append(ids, id)
		mu.Unlock()
		return nil
	}
	conf, err := s.OnChangeAvailability(&core.ChangeAvailabilityRequest{ConnectorId: 0, Type: core.AvailabilityTypeInoperative})
	if err != nil || conf.Status != core.AvailabilityStatusAccepted {
		t.Fatalf("change availability=%#v err=%v", conf, err)
	}
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	got := len(ids)
	mu.Unlock()
	if got != 2 {
		t.Fatalf("connector 0 did not target all connectors: %v", ids)
	}
	if _, err := s.execute([]string{"use", "2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.execute([]string{"plug"}); err != nil {
		t.Fatal(err)
	}
	c2, _ := s.connector(2)
	c2.mu.RLock()
	status := c2.status
	c2.mu.RUnlock()
	if status != core.ChargePointStatusPreparing {
		t.Fatalf("selected connector command status=%s", status)
	}
}

func TestParseAndSchedulerCompatibility(t *testing.T) {
	if got, ok := parseStatus("suspendedevse"); !ok || got != core.ChargePointStatusSuspendedEVSE {
		t.Fatal("case-insensitive status parse failed")
	}
	if _, ok := parseErrorCode("bad"); ok {
		t.Fatal("unknown error code accepted")
	}
	if _, err := ocppMeterWh(-1); err == nil {
		t.Fatal("negative meter accepted")
	}
	if got, err := ocppMeterWh(100000 + 6*15*7.4*1000/3600); err != nil || got != 100185 {
		t.Fatalf("rounded meter=%d err=%v", got, err)
	}
	if _, err := normalizeWebSocketURL("https://ocpp.example.com"); err == nil {
		t.Fatal("https URL accepted")
	}
	s := testSimulator(1)
	s.heartbeatAction = func(<-chan struct{}) error { return errors.New("ignored") }
	s.startHeartbeat(time.Hour)
	s.stopHeartbeat()
}
