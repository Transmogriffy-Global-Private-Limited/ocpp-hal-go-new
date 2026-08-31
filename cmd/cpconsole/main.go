package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	ocpp16 "github.com/lorenzodonini/ocpp-go/ocpp1.6"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/firmware"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/remotetrigger"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
)

const maxOCPPMeterWh int64 = 2147483647

var (
	errWorkerStopped        = errors.New("automatic worker stopped")
	errAutomaticMeterPaused = errors.New("automatic meter paused by connector state")
)

const helpText = `Commands:
  use <connector>                           select a connector (default: -connector)
  state [all|connector]                     show charge-point or connector state
  heartbeat                                 send charge-point Heartbeat
  plug | unplug | finish                    selected connector lifecycle
  authorize <id-tag> | start <id-tag>       selected connector authorization/start
  remote-start [connector]                  execute pending remote start for connector
  tick <seconds> [power-kW] | meter         selected connector metering
  auto <seconds> <power-kW> | auto off      selected connector automatic metering
  suspend ev|evse | resume                  selected connector transaction state
  stop [reason] | remote-stop [connector]   selected/pending connector stop
  fault <error-code> | clear-fault          selected connector fault state
  status <ocpp-status>                      explicit selected connector status
  policy remote-start accept|reject
  policy remote-stop accept|reject
  policy auto-remote on|off
  quit

One cpconsole process is one OCPP charge point and one WebSocket. Connector
commands operate on use <connector>; charge-point commands remain global.`

// connectorState contains only connector-local physical state. Operations are
// serialized per connector, so a slow OCPP call on one connector never blocks
// a separate connector transaction or meter worker.
type connectorState struct {
	id                             int
	ops                            sync.Mutex
	mu                             sync.RWMutex
	status                         core.ChargePointStatus
	errorCode                      core.ChargePointErrorCode
	plugged                        bool
	idTag                          string
	transaction                    int
	meterWh, powerKW, voltage, soc float64
	remoteStart                    *core.RemoteStartTransactionRequest
	remoteStop                     *core.RemoteStopTransactionRequest
	unavailableAfterStop           bool
	autoCancel, autoDone           chan struct{}
	autoInterval                   time.Duration
	autoPowerKW                    float64
	autoTransaction                int
}

type simulator struct {
	cp                                                     ocpp16.ChargePoint
	clientID, model, vendor                                string
	mu                                                     sync.RWMutex
	connectors                                             map[int]*connectorState
	selectedConnector                                      int
	booted, acceptStart, acceptStop, autoRemote            bool
	configuration                                          map[string]string
	automaticMeterInterval                                 time.Duration
	automaticMeterPowerKW                                  float64
	heartbeatMu                                            sync.Mutex
	heartbeatServer, heartbeatOverride, heartbeatEffective int
	heartbeatCancel, heartbeatDone                         chan struct{}

	// Protocol seams are production-backed defaults, while focused tests can
	// capture routing without needing a network connection.
	bootAction      func() (*core.BootNotificationConfirmation, error)
	heartbeatAction func(<-chan struct{}) error
	statusAction    func(int, core.ChargePointErrorCode, core.ChargePointStatus) error
	authorizeAction func(string) (*core.AuthorizeConfirmation, error)
	startAction     func(int, string, int) (*core.StartTransactionConfirmation, error)
	meterAction     func(int, []types.MeterValue, int) error
	stopAction      func(int, int, string, core.Reason) (*core.StopTransactionConfirmation, error)
}

type startupOptions struct {
	connectors, connector      int
	meterStartWh, voltage, soc float64
	heartbeatInterval          int
	autoStartIDTag             string
	autoPowerKW                float64
	autoMeterInterval          int
}

type autoSessionOptions struct {
	connector     int
	idTag         string
	meterInterval time.Duration
	powerKW       float64
}

func newSimulator(id, model, vendor string, count, selected int, meter, voltage, soc float64) *simulator {
	s := &simulator{clientID: id, model: model, vendor: vendor, connectors: make(map[int]*connectorState, count), selectedConnector: selected, acceptStart: true, acceptStop: true, autoRemote: true, configuration: map[string]string{"MeterValueSampleInterval": "60"}}
	for i := 1; i <= count; i++ {
		s.connectors[i] = &connectorState{id: i, status: core.ChargePointStatusUnavailable, errorCode: core.NoError, meterWh: meter, voltage: voltage, soc: soc}
	}
	s.cp = ocpp16.NewChargePoint(id, nil, nil)
	s.cp.SetCoreHandler(s)
	s.cp.SetFirmwareManagementHandler(s)
	s.cp.SetRemoteTriggerHandler(s)
	s.bootAction = func() (*core.BootNotificationConfirmation, error) { return s.cp.BootNotification(s.model, s.vendor) }
	s.heartbeatAction = s.automaticHeartbeat
	s.statusAction = func(id int, code core.ChargePointErrorCode, status core.ChargePointStatus) error {
		_, err := s.cp.StatusNotification(id, code, status, func(r *core.StatusNotificationRequest) { r.Timestamp = types.Now() })
		return err
	}
	s.authorizeAction = func(tag string) (*core.AuthorizeConfirmation, error) {
		return s.cp.Authorize(tag)
	}
	s.startAction = func(id int, tag string, meter int) (*core.StartTransactionConfirmation, error) {
		return s.cp.StartTransaction(id, tag, meter, types.Now())
	}
	s.meterAction = func(id int, values []types.MeterValue, tx int) error {
		_, err := s.cp.MeterValues(id, values, func(r *core.MeterValuesRequest) { r.TransactionId = &tx })
		return err
	}
	s.stopAction = func(meter, tx int, tag string, reason core.Reason) (*core.StopTransactionConfirmation, error) {
		return s.cp.StopTransaction(meter, types.Now(), tx, func(r *core.StopTransactionRequest) { r.IdTag = tag; r.Reason = reason })
	}
	return s
}

func main() {
	urlValue := flag.String("url", env("CP_SIM_URL", "ws://127.0.0.1:18081"), "central-system WebSocket base URL")
	id := flag.String("id", env("CP_SIM_ID", "CP-SIM-001"), "OCPP charge point identity")
	model := flag.String("model", env("CP_SIM_MODEL", "TransEV-Simulator"), "Boot model")
	vendor := flag.String("vendor", env("CP_SIM_VENDOR", "TransEV"), "Boot vendor")
	connectors := flag.Int("connectors", envInt("CP_SIM_CONNECTORS", 1), "number of simulated connectors")
	connector := flag.Int("connector", envInt("CP_SIM_CONNECTOR", 1), "initial selected connector")
	meter := flag.Float64("meter-start-wh", envFloat("CP_SIM_METER_START_WH", 100000), "initial Wh register per connector")
	voltage := flag.Float64("voltage", envFloat("CP_SIM_VOLTAGE", 230), "simulated voltage")
	soc := flag.Float64("soc", envFloat("CP_SIM_SOC", 35), "initial SoC percent")
	heartbeat := flag.Int("heartbeat-interval", envInt("CP_SIM_HEARTBEAT_INTERVAL", 0), "Heartbeat seconds; 0 uses BootNotification")
	autoTag := flag.String("auto-start-id-tag", env("CP_SIM_AUTO_START_ID_TAG", ""), "start one session after boot")
	autoPower := flag.Float64("auto-power-kw", envFloat("CP_SIM_AUTO_POWER_KW", 7.2), "automatic meter power")
	autoInterval := flag.Int("auto-meter-interval", envInt("CP_SIM_AUTO_METER_INTERVAL", 0), "automatic meter seconds; 0 disables")
	flag.Parse()
	opts := startupOptions{connectors: *connectors, connector: *connector, meterStartWh: *meter, voltage: *voltage, soc: *soc, heartbeatInterval: *heartbeat, autoStartIDTag: strings.TrimSpace(*autoTag), autoPowerKW: *autoPower, autoMeterInterval: *autoInterval}
	if err := opts.validate(); err != nil {
		log.Fatal(err)
	}
	if strings.TrimSpace(*id) == "" || strings.TrimSpace(*model) == "" || len(*model) > 20 || strings.TrimSpace(*vendor) == "" || len(*vendor) > 20 {
		log.Fatal("charger ID must be non-empty; OCPP model and vendor must contain 1 to 20 characters")
	}
	base, err := normalizeWebSocketURL(*urlValue)
	if err != nil {
		log.Fatal(err)
	}
	s := newSimulator(*id, *model, *vendor, opts.connectors, opts.connector, opts.meterStartWh, opts.voltage, opts.soc)
	s.setHeartbeatOverride(opts.heartbeatInterval)
	s.setAutomaticMeterConfig(time.Duration(opts.autoMeterInterval)*time.Second, opts.autoPowerKW)
	if err = s.connectAndBoot(base); err != nil {
		log.Fatal(err)
	}
	defer s.close()
	if opts.autoStartIDTag != "" {
		if err = s.startAutomaticSession(autoSessionOptions{connector: opts.connector, idTag: opts.autoStartIDTag, meterInterval: time.Duration(opts.autoMeterInterval) * time.Second, powerKW: opts.autoPowerKW}); err != nil {
			fmt.Printf("[SIM] startup automation failed: %v\n", err)
		}
	}
	fmt.Printf("\nOCPP 1.6J charge point %s (%d connectors) is ready. Type help for commands.\n", *id, opts.connectors)
	if err = runConsole(os.Stdin, os.Stdout, s); err != nil {
		log.Fatal(err)
	}
}

func (o startupOptions) validate() error {
	if o.connectors < 1 || o.connector < 1 || o.connector > o.connectors || o.meterStartWh < 0 || o.voltage <= 0 || o.soc < 0 || o.soc > 100 {
		return errors.New("connectors must be >= 1, selected connector must exist, meter/voltage must be valid, and SoC must be between 0 and 100")
	}
	if o.heartbeatInterval < 0 || o.autoMeterInterval < 0 {
		return errors.New("heartbeat and automatic meter intervals must be zero or greater")
	}
	if o.autoStartIDTag != "" && len(o.autoStartIDTag) > 20 {
		return errors.New("automatic start id-tag must contain 1 to 20 characters")
	}
	if o.autoMeterInterval > 0 && o.autoPowerKW <= 0 {
		return errors.New("automatic meter power must be greater than zero")
	}
	return nil
}

func (s *simulator) connector(id int) (*connectorState, error) {
	s.mu.RLock()
	c := s.connectors[id]
	s.mu.RUnlock()
	if c == nil {
		return nil, fmt.Errorf("connector %d is not configured", id)
	}
	return c, nil
}
func (s *simulator) connectorIDs() []int {
	s.mu.RLock()
	ids := make([]int, 0, len(s.connectors))
	for id := range s.connectors {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	sort.Ints(ids)
	return ids
}
func (s *simulator) selected() (*connectorState, error) {
	s.mu.RLock()
	id := s.selectedConnector
	s.mu.RUnlock()
	return s.connector(id)
}
func (s *simulator) selectConnector(id int) error {
	if _, err := s.connector(id); err != nil {
		return err
	}
	s.mu.Lock()
	s.selectedConnector = id
	s.mu.Unlock()
	return nil
}

func (s *simulator) connectAndBoot(base string) error {
	if err := s.cp.Start(base); err != nil {
		return fmt.Errorf("connect %s to %s: %w", s.clientID, base, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !s.cp.IsConnected() && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if !s.cp.IsConnected() {
		return errors.New("charge point connection did not become ready within 10s")
	}
	if err := s.boot(); err != nil {
		return err
	}
	for _, id := range s.connectorIDs() {
		c, _ := s.connector(id)
		if err := s.setStatus(c, core.ChargePointStatusAvailable, core.NoError); err != nil {
			return err
		}
	}
	return nil
}
func (s *simulator) close() {
	for _, id := range s.connectorIDs() {
		c, _ := s.connector(id)
		c.stopAutoAndWait()
	}
	s.stopHeartbeat()
	s.cp.Stop()
}
func (s *simulator) boot() error {
	conf, err := s.bootAction()
	if err != nil {
		return fmt.Errorf("BootNotification: %w", err)
	}
	if conf == nil || conf.Status != core.RegistrationStatusAccepted {
		status := "nil"
		if conf != nil {
			status = string(conf.Status)
		}
		return fmt.Errorf("BootNotification was not accepted: %s", status)
	}
	if err = s.configureBootHeartbeat(conf.Interval); err != nil {
		return err
	}
	s.mu.Lock()
	s.booted = true
	s.mu.Unlock()
	fmt.Printf("[OCPP] BootNotification accepted; heartbeat interval=%ds\n", conf.Interval)
	return nil
}

func selectHeartbeatInterval(server, override int) (int, error) {
	if server <= 0 {
		return 0, errors.New("BootNotification returned an invalid heartbeat interval")
	}
	if override > 0 {
		return override, nil
	}
	return server, nil
}
func (s *simulator) setHeartbeatOverride(seconds int) {
	s.heartbeatMu.Lock()
	s.heartbeatOverride = seconds
	s.heartbeatMu.Unlock()
}
func (s *simulator) setAutomaticMeterConfig(interval time.Duration, powerKW float64) {
	s.mu.Lock()
	s.automaticMeterInterval, s.automaticMeterPowerKW = interval, powerKW
	s.mu.Unlock()
}
func (s *simulator) configureBootHeartbeat(server int) error {
	s.heartbeatMu.Lock()
	override := s.heartbeatOverride
	s.heartbeatMu.Unlock()
	effective, err := selectHeartbeatInterval(server, override)
	if err != nil {
		return err
	}
	s.heartbeatMu.Lock()
	s.heartbeatServer, s.heartbeatEffective = server, effective
	s.heartbeatMu.Unlock()
	s.mu.Lock()
	s.configuration["HeartbeatInterval"] = strconv.Itoa(effective)
	s.mu.Unlock()
	s.startHeartbeat(time.Duration(effective) * time.Second)
	return nil
}
func (s *simulator) startHeartbeat(interval time.Duration) {
	s.stopHeartbeat()
	if interval <= 0 {
		return
	}
	cancel, done := make(chan struct{}), make(chan struct{})
	s.heartbeatMu.Lock()
	s.heartbeatCancel, s.heartbeatDone = cancel, done
	action := s.heartbeatAction
	s.heartbeatMu.Unlock()
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := action(cancel); err != nil && !errors.Is(err, errWorkerStopped) {
					fmt.Printf("[SIM] automatic heartbeat failed: %v\n", err)
				}
			case <-cancel:
				return
			}
		}
	}()
}
func (s *simulator) stopHeartbeat() {
	s.heartbeatMu.Lock()
	cancel, done := s.heartbeatCancel, s.heartbeatDone
	s.heartbeatCancel, s.heartbeatDone = nil, nil
	s.heartbeatMu.Unlock()
	if cancel != nil {
		close(cancel)
		<-done
	}
}
func (s *simulator) heartbeat() error { return s.heartbeatLocked() }
func (s *simulator) heartbeatLocked() error {
	conf, err := s.cp.Heartbeat()
	if err != nil {
		return fmt.Errorf("Heartbeat: %w", err)
	}
	fmt.Printf("[OCPP] Heartbeat accepted; central time=%s\n", conf.CurrentTime.String())
	return nil
}
func (s *simulator) automaticHeartbeat(cancel <-chan struct{}) error {
	select {
	case <-cancel:
		return errWorkerStopped
	default:
		return s.heartbeatLocked()
	}
}

func runConsole(in io.Reader, out io.Writer, s *simulator) error {
	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, "cp> ")
		if !scanner.Scan() {
			return scanner.Err()
		}
		quit, err := s.execute(strings.Fields(scanner.Text()))
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
		}
		if quit {
			return nil
		}
	}
}

func (s *simulator) execute(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if strings.EqualFold(args[0], "use") {
		if len(args) != 2 {
			return false, errors.New("usage: use <connector>")
		}
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return false, errors.New("connector must be a positive integer")
		}
		if err = s.selectConnector(id); err != nil {
			return false, err
		}
		fmt.Printf("[SIM] selected connector %d\n", id)
		return false, nil
	}
	switch strings.ToLower(args[0]) {
	case "help", "?":
		fmt.Println(helpText)
	case "state":
		return false, s.executeState(args)
	case "heartbeat":
		return false, s.heartbeat()
	case "plug", "unplug", "finish", "authorize", "start", "remote-start", "tick", "meter", "auto", "suspend", "resume", "stop", "remote-stop", "fault", "clear-fault", "status":
		return false, s.executeConnectorCommand(args)
	case "policy":
		return false, s.executePolicy(args)
	case "quit", "exit":
		return true, nil
	default:
		return false, fmt.Errorf("unknown command %q; type help", args[0])
	}
	return false, nil
}

func (s *simulator) executeState(args []string) error {
	if len(args) > 2 {
		return errors.New("usage: state [all|connector]")
	}
	if len(args) == 2 && !strings.EqualFold(args[1], "all") {
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return errors.New("usage: state [all|connector]")
		}
		c, err := s.connector(id)
		if err != nil {
			return err
		}
		s.printConnectorState(c)
		return nil
	}
	s.printState()
	if len(args) == 2 {
		for _, id := range s.connectorIDs() {
			c, _ := s.connector(id)
			s.printConnectorState(c)
		}
	}
	return nil
}

func (s *simulator) executeConnectorCommand(args []string) error {
	c, err := s.selected()
	if err != nil {
		return err
	}
	command := strings.ToLower(args[0])
	switch command {
	case "plug":
		if len(args) != 1 {
			return errors.New("usage: plug")
		}
		return s.plug(c)
	case "unplug":
		if len(args) != 1 {
			return errors.New("usage: unplug")
		}
		return s.unplug(c)
	case "finish":
		if len(args) != 1 {
			return errors.New("usage: finish")
		}
		return s.finish(c)
	case "authorize":
		if len(args) != 2 {
			return errors.New("usage: authorize <id-tag>")
		}
		return s.authorize(c, args[1])
	case "start":
		if len(args) != 2 {
			return errors.New("usage: start <id-tag>")
		}
		return s.startTransaction(c, args[1], false)
	case "remote-start":
		if len(args) == 2 {
			id, e := strconv.Atoi(args[1])
			if e != nil {
				return errors.New("usage: remote-start [connector]")
			}
			c, err = s.connector(id)
			if err != nil {
				return err
			}
		} else if len(args) != 1 {
			return errors.New("usage: remote-start [connector]")
		}
		return s.executePendingRemoteStart(c)
	case "tick":
		return s.executeTick(c, args)
	case "meter":
		if len(args) != 1 {
			return errors.New("usage: meter")
		}
		return s.sendMeter(c, 0, s.currentPower(c))
	case "auto":
		return s.executeAuto(c, args)
	case "suspend":
		if len(args) != 2 {
			return errors.New("usage: suspend ev|evse")
		}
		if strings.EqualFold(args[1], "ev") {
			return s.requireTransactionStatus(c, core.ChargePointStatusSuspendedEV)
		}
		if strings.EqualFold(args[1], "evse") {
			return s.requireTransactionStatus(c, core.ChargePointStatusSuspendedEVSE)
		}
		return errors.New("usage: suspend ev|evse")
	case "resume":
		if len(args) != 1 {
			return errors.New("usage: resume")
		}
		return s.requireTransactionStatus(c, core.ChargePointStatusCharging)
	case "stop":
		reason := core.ReasonLocal
		if len(args) > 2 {
			return errors.New("usage: stop [reason]")
		}
		if len(args) == 2 {
			var ok bool
			reason, ok = parseStopReason(args[1])
			if !ok {
				return fmt.Errorf("invalid stop reason %q", args[1])
			}
		}
		return s.stopTransaction(c, reason)
	case "remote-stop":
		if len(args) == 2 {
			id, e := strconv.Atoi(args[1])
			if e != nil {
				return errors.New("usage: remote-stop [connector]")
			}
			c, err = s.connector(id)
			if err != nil {
				return err
			}
		} else if len(args) != 1 {
			return errors.New("usage: remote-stop [connector]")
		}
		return s.executePendingRemoteStop(c)
	case "fault":
		if len(args) != 2 {
			return errors.New("usage: fault <error-code>")
		}
		code, ok := parseErrorCode(args[1])
		if !ok || code == core.NoError {
			return fmt.Errorf("invalid fault code %q", args[1])
		}
		return s.setStatus(c, core.ChargePointStatusFaulted, code)
	case "clear-fault":
		if len(args) != 1 {
			return errors.New("usage: clear-fault")
		}
		c.mu.RLock()
		plugged := c.plugged
		c.mu.RUnlock()
		if plugged {
			return s.setStatus(c, core.ChargePointStatusPreparing, core.NoError)
		}
		return s.setStatus(c, core.ChargePointStatusAvailable, core.NoError)
	case "status":
		if len(args) != 2 {
			return errors.New("usage: status <ocpp-status>")
		}
		status, ok := parseStatus(args[1])
		if !ok {
			return fmt.Errorf("invalid OCPP status %q", args[1])
		}
		return s.setStatus(c, status, core.NoError)
	}
	return fmt.Errorf("unknown connector command %q", command)
}

func (s *simulator) executeTick(c *connectorState, args []string) error {
	if len(args) < 2 || len(args) > 3 {
		return errors.New("usage: tick <seconds> [power-kW]")
	}
	seconds, err := strconv.ParseFloat(args[1], 64)
	if err != nil || seconds <= 0 {
		return errors.New("seconds must be greater than zero")
	}
	power := s.currentPower(c)
	if len(args) == 3 {
		power, err = strconv.ParseFloat(args[2], 64)
		if err != nil || power < 0 {
			return errors.New("power-kW must be zero or greater")
		}
	}
	if power == 0 {
		power = 7.2
	}
	return s.sendMeter(c, time.Duration(seconds*float64(time.Second)), power)
}
func (s *simulator) executeAuto(c *connectorState, args []string) error {
	if len(args) == 2 && strings.EqualFold(args[1], "off") {
		c.stopAutoAndWait()
		fmt.Printf("[SIM] connector %d automatic metering stopped\n", c.id)
		return nil
	}
	if len(args) != 3 {
		return errors.New("usage: auto <seconds> <power-kW> | auto off")
	}
	seconds, err := strconv.ParseFloat(args[1], 64)
	if err != nil || seconds <= 0 {
		return errors.New("seconds must be greater than zero")
	}
	power, err := strconv.ParseFloat(args[2], 64)
	if err != nil || power <= 0 {
		return errors.New("power-kW must be greater than zero")
	}
	return s.startAuto(c, time.Duration(seconds*float64(time.Second)), power)
}
func (s *simulator) startAutomaticSession(o autoSessionOptions) error {
	c, err := s.connector(o.connector)
	if err != nil {
		return err
	}
	return runAutomaticSession(o, func() error { return s.plug(c) }, func() error { return s.startTransaction(c, o.idTag, false) }, func() error {
		return nil // startTransaction owns configured transaction-bound automatic metering.
	})
}
func runAutomaticSession(_ autoSessionOptions, plug, start, meter func() error) error {
	if err := plug(); err != nil {
		return fmt.Errorf("auto-start plug: %w", err)
	}
	if err := start(); err != nil {
		return fmt.Errorf("auto-start transaction: %w", err)
	}
	if err := meter(); err != nil {
		return fmt.Errorf("auto-start metering: %w", err)
	}
	return nil
}

func (s *simulator) executePolicy(args []string) error {
	if len(args) != 3 {
		return errors.New("usage: policy remote-start accept|reject | policy remote-stop accept|reject | policy auto-remote on|off")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch strings.ToLower(args[1]) {
	case "remote-start":
		v, ok := parseBoolPolicy(args[2], "accept", "reject")
		if !ok {
			return errors.New("remote-start policy must be accept or reject")
		}
		s.acceptStart = v
	case "remote-stop":
		v, ok := parseBoolPolicy(args[2], "accept", "reject")
		if !ok {
			return errors.New("remote-stop policy must be accept or reject")
		}
		s.acceptStop = v
	case "auto-remote":
		v, ok := parseBoolPolicy(args[2], "on", "off")
		if !ok {
			return errors.New("auto-remote policy must be on or off")
		}
		s.autoRemote = v
	default:
		return fmt.Errorf("unknown policy %q", args[1])
	}
	fmt.Printf("[SIM] policy %s=%s\n", args[1], strings.ToLower(args[2]))
	return nil
}

func (s *simulator) plug(c *connectorState) error {
	c.ops.Lock()
	defer c.ops.Unlock()
	c.mu.Lock()
	if c.transaction != 0 {
		c.mu.Unlock()
		return errors.New("connector already has an active transaction")
	}
	c.plugged = true
	c.mu.Unlock()
	return s.setStatusLocked(c, core.ChargePointStatusPreparing, core.NoError)
}
func (s *simulator) unplug(c *connectorState) error {
	c.ops.Lock()
	defer c.ops.Unlock()
	c.mu.Lock()
	if c.transaction != 0 {
		c.mu.Unlock()
		return errors.New("stop the transaction before unplugging")
	}
	c.plugged = false
	c.mu.Unlock()
	return s.setStatusLocked(c, core.ChargePointStatusAvailable, core.NoError)
}
func (s *simulator) finish(c *connectorState) error {
	c.ops.Lock()
	defer c.ops.Unlock()
	c.mu.RLock()
	if c.transaction != 0 {
		c.mu.RUnlock()
		return errors.New("stop the active transaction before finish")
	}
	target := core.ChargePointStatusAvailable
	if c.unavailableAfterStop {
		target = core.ChargePointStatusUnavailable
	}
	c.mu.RUnlock()
	return s.setStatusLocked(c, target, core.NoError)
}
func (s *simulator) authorize(c *connectorState, tag string) error {
	c.ops.Lock()
	defer c.ops.Unlock()
	return s.authorizeLocked(tag)
}
func (s *simulator) authorizeLocked(tag string) error {
	if tag == "" || len(tag) > 20 {
		return errors.New("id-tag must contain 1 to 20 characters")
	}
	conf, err := s.authorizeAction(tag)
	if err != nil {
		return fmt.Errorf("Authorize: %w", err)
	}
	if conf == nil {
		return errors.New("Authorize returned an empty confirmation")
	}
	fmt.Printf("[OCPP] Authorize idTag=%s status=%s\n", tag, conf.IdTagInfo.Status)
	if conf.IdTagInfo.Status != types.AuthorizationStatusAccepted {
		return fmt.Errorf("id-tag was not accepted: %s", conf.IdTagInfo.Status)
	}
	return nil
}

func (s *simulator) startTransaction(c *connectorState, tag string, remote bool) error {
	c.ops.Lock()
	defer c.ops.Unlock()
	if tag == "" || len(tag) > 20 {
		return errors.New("id-tag must contain 1 to 20 characters")
	}
	c.mu.RLock()
	active, status, meter := c.transaction != 0, c.status, c.meterWh
	c.mu.RUnlock()
	if active {
		return errors.New("a transaction is already active on this connector")
	}
	if status == core.ChargePointStatusFaulted || status == core.ChargePointStatusUnavailable {
		return fmt.Errorf("cannot start while connector status is %s", status)
	}
	if !remote {
		if err := s.authorizeLocked(tag); err != nil {
			return err
		}
	}
	if status == core.ChargePointStatusAvailable {
		c.mu.Lock()
		c.plugged = true
		c.mu.Unlock()
		if err := s.setStatusLocked(c, core.ChargePointStatusPreparing, core.NoError); err != nil {
			return err
		}
	}
	ocppMeter, err := ocppMeterWh(meter)
	if err != nil {
		return fmt.Errorf("start meter: %w", err)
	}
	conf, err := s.startAction(c.id, tag, ocppMeter)
	if err != nil {
		return fmt.Errorf("StartTransaction: %w", err)
	}
	if conf == nil || conf.IdTagInfo.Status != types.AuthorizationStatusAccepted || conf.TransactionId <= 0 {
		state := "nil"
		if conf != nil {
			state = string(conf.IdTagInfo.Status)
		}
		return fmt.Errorf("StartTransaction was not accepted: %s", state)
	}
	c.mu.Lock()
	c.transaction, c.idTag, c.remoteStart = conf.TransactionId, tag, nil
	c.mu.Unlock()
	if err := s.setStatusLocked(c, core.ChargePointStatusCharging, core.NoError); err != nil {
		return err
	}
	s.mu.RLock()
	interval, powerKW := s.automaticMeterInterval, s.automaticMeterPowerKW
	s.mu.RUnlock()
	if interval > 0 {
		if err := s.startAuto(c, interval, powerKW); err != nil {
			fmt.Printf("[SIM] automatic metering did not start for connector %d transactionId=%d: %v\n", c.id, conf.TransactionId, err)
		}
	}
	fmt.Printf("[OCPP] StartTransaction connector=%d transactionId=%d meterStart=%.0fWh idTag=%s\n", c.id, conf.TransactionId, meter, tag)
	return nil
}

func (s *simulator) sendMeter(c *connectorState, elapsed time.Duration, power float64) error {
	c.ops.Lock()
	defer c.ops.Unlock()
	return s.sendMeterLocked(c, elapsed, power)
}
func (s *simulator) sendAutomaticMeter(c *connectorState, cancel <-chan struct{}, elapsed time.Duration, power float64, expectedTransaction int) error {
	select {
	case <-cancel:
		return errWorkerStopped
	default:
	}
	c.ops.Lock()
	defer c.ops.Unlock()
	select {
	case <-cancel:
		return errWorkerStopped
	default:
	}
	c.mu.RLock()
	charging := c.status == core.ChargePointStatusCharging
	transactionMatches := c.transaction == expectedTransaction && c.autoTransaction == expectedTransaction
	c.mu.RUnlock()
	if !transactionMatches {
		return errWorkerStopped
	}
	if !charging {
		return errAutomaticMeterPaused
	}
	return s.sendMeterLocked(c, elapsed, power)
}
func (s *simulator) sendMeterLocked(c *connectorState, elapsed time.Duration, power float64) error {
	c.mu.Lock()
	if c.transaction == 0 {
		c.mu.Unlock()
		return errors.New("meter values require an active transaction")
	}
	if elapsed < 0 || power < 0 {
		c.mu.Unlock()
		return errors.New("elapsed time and power cannot be negative")
	}
	delta := power * 1000 * elapsed.Hours()
	c.meterWh += delta
	c.powerKW = power
	if delta > 0 && c.soc < 100 {
		c.soc += delta / 60000 * 100
		if c.soc > 100 {
			c.soc = 100
		}
	}
	meter, tx, voltage, soc := c.meterWh, c.transaction, c.voltage, c.soc
	c.mu.Unlock()
	ocppMeter, err := ocppMeterWh(meter)
	if err != nil {
		return fmt.Errorf("meter value: %w", err)
	}
	current := 0.0
	if voltage > 0 {
		current = power * 1000 / voltage
	}
	values := []types.MeterValue{{Timestamp: types.Now(), SampledValue: []types.SampledValue{{Value: strconv.Itoa(ocppMeter), Context: types.ReadingContextSamplePeriodic, Measurand: types.MeasurandEnergyActiveImportRegister, Unit: types.UnitOfMeasureWh}, {Value: fmt.Sprintf("%.3f", power), Context: types.ReadingContextSamplePeriodic, Measurand: types.MeasurandPowerActiveImport, Unit: types.UnitOfMeasureKW}, {Value: fmt.Sprintf("%.2f", current), Context: types.ReadingContextSamplePeriodic, Measurand: types.MeasurandCurrentImport, Unit: types.UnitOfMeasureA}, {Value: fmt.Sprintf("%.1f", voltage), Context: types.ReadingContextSamplePeriodic, Measurand: types.MeasurandVoltage, Unit: types.UnitOfMeasureV}, {Value: fmt.Sprintf("%.1f", soc), Context: types.ReadingContextSamplePeriodic, Measurand: types.MeasurandSoC, Unit: types.UnitOfMeasurePercent}}}}
	if err := s.meterAction(c.id, values, tx); err != nil {
		return fmt.Errorf("MeterValues: %w", err)
	}
	fmt.Printf("[OCPP] MeterValues connector=%d transactionId=%d meter=%dWh (+%.1fWh) power=%.3fkW current=%.2fA voltage=%.1fV soc=%.1f%%\n", c.id, tx, ocppMeter, delta, power, current, voltage, soc)
	return nil
}

func (s *simulator) stopTransaction(c *connectorState, reason core.Reason) error {
	c.stopAutoAndWait()
	c.ops.Lock()
	defer c.ops.Unlock()
	c.mu.RLock()
	tx, meter, tag := c.transaction, c.meterWh, c.idTag
	c.mu.RUnlock()
	if tx == 0 {
		return errors.New("no active transaction")
	}
	ocppMeter, err := ocppMeterWh(meter)
	if err != nil {
		return fmt.Errorf("stop meter: %w", err)
	}
	conf, err := s.stopAction(ocppMeter, tx, tag, reason)
	if err != nil {
		return fmt.Errorf("StopTransaction: %w", err)
	}
	state := "Accepted"
	if conf != nil && conf.IdTagInfo != nil {
		state = string(conf.IdTagInfo.Status)
	}
	c.mu.Lock()
	c.transaction, c.idTag, c.powerKW, c.remoteStop = 0, "", 0, nil
	c.mu.Unlock()
	if err := s.setStatusLocked(c, core.ChargePointStatusFinishing, core.NoError); err != nil {
		return err
	}
	fmt.Printf("[OCPP] StopTransaction connector=%d status=%s transactionId=%d meterStop=%.0fWh reason=%s\n", c.id, state, tx, meter, reason)
	return nil
}
func ocppMeterWh(value float64) (int, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, errors.New("meter must be a finite non-negative value")
	}
	rounded := math.Round(value)
	if rounded > float64(maxOCPPMeterWh) {
		return 0, fmt.Errorf("meter exceeds OCPP integer range (%d Wh)", maxOCPPMeterWh)
	}
	return int(rounded), nil
}
func (s *simulator) setStatus(c *connectorState, status core.ChargePointStatus, code core.ChargePointErrorCode) error {
	c.ops.Lock()
	defer c.ops.Unlock()
	return s.setStatusLocked(c, status, code)
}
func (s *simulator) setStatusLocked(c *connectorState, status core.ChargePointStatus, code core.ChargePointErrorCode) error {
	if err := s.statusAction(c.id, code, status); err != nil {
		return fmt.Errorf("StatusNotification %s: %w", status, err)
	}
	c.mu.Lock()
	c.status, c.errorCode = status, code
	c.mu.Unlock()
	fmt.Printf("[OCPP] StatusNotification connector=%d status=%s errorCode=%s\n", c.id, status, code)
	return nil
}
func (s *simulator) requireTransactionStatus(c *connectorState, status core.ChargePointStatus) error {
	c.mu.RLock()
	transaction := c.transaction
	c.mu.RUnlock()
	if transaction == 0 {
		return errors.New("this status requires an active transaction")
	}
	return s.setStatus(c, status, core.NoError)
}
func (s *simulator) currentPower(c *connectorState) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.powerKW
}

func (s *simulator) startAuto(c *connectorState, interval time.Duration, power float64) error {
	c.mu.RLock()
	transaction := c.transaction
	c.mu.RUnlock()
	if transaction == 0 {
		return errors.New("automatic metering requires an active transaction")
	}
	c.stopAutoAndWait()
	cancel, done := make(chan struct{}), make(chan struct{})
	c.mu.Lock()
	c.autoCancel, c.autoDone, c.autoInterval, c.autoPowerKW, c.autoTransaction = cancel, done, interval, power, transaction
	c.mu.Unlock()
	go func() {
		defer close(done)
		defer c.clearAuto(cancel)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		lastSampleAt := time.Now()
		for {
			select {
			case sampleAt := <-ticker.C:
				elapsed := sampleAt.Sub(lastSampleAt)
				lastSampleAt = sampleAt
				err := s.sendAutomaticMeter(c, cancel, elapsed, power, transaction)
				if errors.Is(err, errWorkerStopped) {
					return
				}
				if errors.Is(err, errAutomaticMeterPaused) {
					continue
				}
				if err != nil {
					fmt.Printf("[SIM] connector %d automatic meter stopped: %v\n", c.id, err)
					return
				}
			case <-cancel:
				return
			}
		}
	}()
	fmt.Printf("[SIM] connector %d automatic metering transactionId=%d every %s at %.3fkW\n", c.id, transaction, interval, power)
	return nil
}
func (c *connectorState) stopAutoAndWait() {
	c.mu.Lock()
	cancel, done := c.autoCancel, c.autoDone
	c.autoCancel, c.autoDone, c.autoInterval, c.autoPowerKW, c.autoTransaction = nil, nil, 0, 0, 0
	c.mu.Unlock()
	if cancel != nil {
		close(cancel)
		<-done
	}
}
func (c *connectorState) clearAuto(cancel chan struct{}) {
	c.mu.Lock()
	if c.autoCancel == cancel {
		c.autoCancel, c.autoDone, c.autoInterval, c.autoPowerKW, c.autoTransaction = nil, nil, 0, 0, 0
	}
	c.mu.Unlock()
}

func (s *simulator) remoteStartEligible(c *connectorState) bool {
	c.mu.RLock()
	ok := c.transaction == 0 && c.remoteStart == nil && (c.status == core.ChargePointStatusAvailable || c.status == core.ChargePointStatusPreparing)
	c.mu.RUnlock()
	return ok
}
func (s *simulator) selectRemoteStartConnector(r *core.RemoteStartTransactionRequest) (*connectorState, error) {
	if r == nil {
		return nil, errors.New("remote start request is required")
	}
	if r.ConnectorId != nil {
		c, err := s.connector(*r.ConnectorId)
		if err != nil {
			return nil, err
		}
		if !s.remoteStartEligible(c) {
			return nil, fmt.Errorf("connector %d is not eligible for remote start", c.id)
		}
		return c, nil
	}
	for _, id := range s.connectorIDs() {
		c, _ := s.connector(id)
		if s.remoteStartEligible(c) {
			return c, nil
		}
	}
	return nil, errors.New("no configured connector is eligible for remote start")
}
func (s *simulator) findTransactionConnector(tx int) (*connectorState, error) {
	for _, id := range s.connectorIDs() {
		c, _ := s.connector(id)
		c.mu.RLock()
		match := c.transaction == tx
		c.mu.RUnlock()
		if match {
			return c, nil
		}
	}
	return nil, fmt.Errorf("transaction %d is not active on this charge point", tx)
}
func (s *simulator) executePendingRemoteStart(c *connectorState) error {
	c.mu.RLock()
	r := c.remoteStart
	c.mu.RUnlock()
	if r == nil {
		return errors.New("no accepted remote start is pending on this connector")
	}
	return s.startTransaction(c, r.IdTag, true)
}
func (s *simulator) executePendingRemoteStop(c *connectorState) error {
	c.mu.RLock()
	r := c.remoteStop
	c.mu.RUnlock()
	if r == nil {
		return errors.New("no accepted remote stop is pending on this connector")
	}
	if err := s.stopTransaction(c, core.ReasonRemote); err != nil {
		return err
	}
	return s.finish(c)
}

func (s *simulator) printState() {
	s.mu.RLock()
	selected, booted, start, stop, auto := s.selectedConnector, s.booted, s.acceptStart, s.acceptStop, s.autoRemote
	s.mu.RUnlock()
	s.heartbeatMu.Lock()
	server, effective, override, active := s.heartbeatServer, s.heartbeatEffective, s.heartbeatOverride > 0, s.heartbeatCancel != nil
	s.heartbeatMu.Unlock()
	fmt.Printf("connected=%t booted=%t id=%s connectors=%d selected=%d heartbeat server=%ds effective=%ds override=%t active=%t remoteStart=%t remoteStop=%t autoRemote=%t\n", s.cp.IsConnected(), booted, s.clientID, len(s.connectorIDs()), selected, server, effective, override, active, start, stop, auto)
}
func (s *simulator) printConnectorState(c *connectorState) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	fmt.Printf("connector=%d status=%s error=%s plugged=%t transactionId=%d idTag=%q meter=%.0fWh power=%.3fkW voltage=%.1fV soc=%.1f%% pendingRemoteStart=%t pendingRemoteStop=%t autoMeter=%t interval=%s autoPower=%.3fkW\n", c.id, c.status, c.errorCode, c.plugged, c.transaction, c.idTag, c.meterWh, c.powerKW, c.voltage, c.soc, c.remoteStart != nil, c.remoteStop != nil, c.autoCancel != nil, c.autoInterval, c.autoPowerKW)
}

// OCPP Core profile: Central System -> Charge Point handlers.
func (s *simulator) OnChangeAvailability(r *core.ChangeAvailabilityRequest) (*core.ChangeAvailabilityConfirmation, error) {
	if r == nil {
		return core.NewChangeAvailabilityConfirmation(core.AvailabilityStatusRejected), nil
	}
	fmt.Printf("[REMOTE] ChangeAvailability connector=%d type=%s\n", r.ConnectorId, r.Type)
	targets := make([]*connectorState, 0)
	if r.ConnectorId == 0 {
		for _, id := range s.connectorIDs() {
			c, _ := s.connector(id)
			targets = append(targets, c)
		}
	} else {
		c, err := s.connector(r.ConnectorId)
		if err != nil {
			return core.NewChangeAvailabilityConfirmation(core.AvailabilityStatusRejected), nil
		}
		targets = append(targets, c)
	}
	scheduled := false
	for _, c := range targets {
		c.mu.Lock()
		active := c.transaction != 0
		if r.Type == core.AvailabilityTypeInoperative && active {
			c.unavailableAfterStop = true
			scheduled = true
		}
		if r.Type == core.AvailabilityTypeOperative {
			c.unavailableAfterStop = false
		}
		c.mu.Unlock()
	}
	if scheduled {
		return core.NewChangeAvailabilityConfirmation(core.AvailabilityStatusScheduled), nil
	}
	status := core.ChargePointStatusAvailable
	if r.Type == core.AvailabilityTypeInoperative {
		status = core.ChargePointStatusUnavailable
	}
	for _, c := range targets {
		go func(c *connectorState) {
			if err := s.setStatus(c, status, core.NoError); err != nil {
				fmt.Printf("[SIM] ChangeAvailability connector %d failed: %v\n", c.id, err)
			}
		}(c)
	}
	return core.NewChangeAvailabilityConfirmation(core.AvailabilityStatusAccepted), nil
}
func (s *simulator) OnChangeConfiguration(r *core.ChangeConfigurationRequest) (*core.ChangeConfigurationConfirmation, error) {
	if r == nil {
		return core.NewChangeConfigurationConfirmation(core.ConfigurationStatusRejected), nil
	}
	fmt.Printf("[REMOTE] ChangeConfiguration key=%s value=%s\n", r.Key, r.Value)
	if r.Key == "HeartbeatInterval" {
		interval, err := strconv.Atoi(r.Value)
		if err != nil || interval <= 0 {
			return core.NewChangeConfigurationConfirmation(core.ConfigurationStatusRejected), nil
		}
		s.heartbeatMu.Lock()
		override := s.heartbeatOverride
		s.heartbeatMu.Unlock()
		if override > 0 {
			return core.NewChangeConfigurationConfirmation(core.ConfigurationStatusRejected), nil
		}
		s.heartbeatMu.Lock()
		s.heartbeatEffective = interval
		s.heartbeatMu.Unlock()
		s.mu.Lock()
		s.configuration[r.Key] = r.Value
		s.mu.Unlock()
		s.startHeartbeat(time.Duration(interval) * time.Second)
		return core.NewChangeConfigurationConfirmation(core.ConfigurationStatusAccepted), nil
	}
	s.mu.Lock()
	s.configuration[r.Key] = r.Value
	s.mu.Unlock()
	return core.NewChangeConfigurationConfirmation(core.ConfigurationStatusAccepted), nil
}
func (s *simulator) OnClearCache(*core.ClearCacheRequest) (*core.ClearCacheConfirmation, error) {
	fmt.Println("[REMOTE] ClearCache")
	return core.NewClearCacheConfirmation(core.ClearCacheStatusAccepted), nil
}
func (s *simulator) OnDataTransfer(*core.DataTransferRequest) (*core.DataTransferConfirmation, error) {
	return core.NewDataTransferConfirmation(core.DataTransferStatusAccepted), nil
}
func (s *simulator) OnGetConfiguration(r *core.GetConfigurationRequest) (*core.GetConfigurationConfirmation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := r.Key
	if len(keys) == 0 {
		keys = make([]string, 0, len(s.configuration))
		for key := range s.configuration {
			keys = append(keys, key)
		}
		sort.Strings(keys)
	}
	known := make([]core.ConfigurationKey, 0, len(keys))
	unknown := make([]string, 0)
	for _, key := range keys {
		value, ok := s.configuration[key]
		if !ok {
			unknown = append(unknown, key)
			continue
		}
		v := value
		known = append(known, core.ConfigurationKey{Key: key, Value: &v})
	}
	conf := core.NewGetConfigurationConfirmation(known)
	conf.UnknownKey = unknown
	return conf, nil
}
func (s *simulator) OnRemoteStartTransaction(r *core.RemoteStartTransactionRequest) (*core.RemoteStartTransactionConfirmation, error) {
	s.mu.RLock()
	accept, auto := s.acceptStart, s.autoRemote
	s.mu.RUnlock()
	c, err := s.selectRemoteStartConnector(r)
	accepted := accept && err == nil
	if accepted {
		c.mu.Lock()
		accepted = c.transaction == 0 && c.remoteStart == nil && (c.status == core.ChargePointStatusAvailable || c.status == core.ChargePointStatusPreparing)
		if accepted {
			c.remoteStart = r
		}
		c.mu.Unlock()
	}
	status := types.RemoteStartStopStatusRejected
	if accepted {
		status = types.RemoteStartStopStatusAccepted
	}
	requested := 0
	if r != nil && r.ConnectorId != nil {
		requested = *r.ConnectorId
	}
	tag := ""
	if r != nil {
		tag = r.IdTag
	}
	fmt.Printf("[REMOTE] RemoteStartTransaction connector=%d idTag=%s response=%s auto=%t\n", requested, tag, status, auto)
	if accepted && auto {
		go func() {
			time.Sleep(100 * time.Millisecond)
			if err := s.executePendingRemoteStart(c); err != nil {
				fmt.Printf("[SIM] automatic remote start connector %d failed: %v\n", c.id, err)
			}
		}()
	}
	return core.NewRemoteStartTransactionConfirmation(status), nil
}
func (s *simulator) OnRemoteStopTransaction(r *core.RemoteStopTransactionRequest) (*core.RemoteStopTransactionConfirmation, error) {
	s.mu.RLock()
	accept := s.acceptStop
	s.mu.RUnlock()
	var c *connectorState
	var err error
	if r == nil {
		err = errors.New("remote stop request is required")
	} else {
		c, err = s.findTransactionConnector(r.TransactionId)
	}
	accepted := accept && err == nil
	if accepted {
		c.mu.Lock()
		if c.transaction != r.TransactionId || c.remoteStop != nil {
			accepted = false
		} else {
			c.remoteStop = r
		}
		c.mu.Unlock()
	}
	status := types.RemoteStartStopStatusRejected
	if accepted {
		status = types.RemoteStartStopStatusAccepted
	}
	tx := 0
	if r != nil {
		tx = r.TransactionId
	}
	fmt.Printf("[REMOTE] RemoteStopTransaction transactionId=%d response=%s completing=%t\n", tx, status, accepted)
	if accepted {
		go func() {
			if err := s.executePendingRemoteStop(c); err != nil {
				fmt.Printf("[SIM] automatic remote stop connector %d failed: %v\n", c.id, err)
			}
		}()
	}
	return core.NewRemoteStopTransactionConfirmation(status), nil
}
func (s *simulator) OnReset(r *core.ResetRequest) (*core.ResetConfirmation, error) {
	fmt.Printf("[REMOTE] Reset type=%s\n", r.Type)
	go func() {
		time.Sleep(100 * time.Millisecond)
		reason := core.ReasonSoftReset
		if r.Type == core.ResetTypeHard {
			reason = core.ReasonHardReset
		}
		for _, id := range s.connectorIDs() {
			c, _ := s.connector(id)
			c.mu.RLock()
			active := c.transaction != 0
			c.mu.RUnlock()
			if active {
				if err := s.stopTransaction(c, reason); err != nil {
					fmt.Printf("[SIM] reset stop connector %d failed: %v\n", id, err)
				}
			}
		}
		if err := s.boot(); err != nil {
			fmt.Printf("[SIM] reset boot failed: %v\n", err)
			return
		}
		for _, id := range s.connectorIDs() {
			c, _ := s.connector(id)
			if err := s.finish(c); err != nil {
				fmt.Printf("[SIM] reset status connector %d failed: %v\n", id, err)
			}
		}
	}()
	return core.NewResetConfirmation(core.ResetStatusAccepted), nil
}
func (s *simulator) OnUnlockConnector(r *core.UnlockConnectorRequest) (*core.UnlockConnectorConfirmation, error) {
	fmt.Printf("[REMOTE] UnlockConnector connector=%d\n", r.ConnectorId)
	if _, err := s.connector(r.ConnectorId); err != nil {
		return core.NewUnlockConnectorConfirmation(core.UnlockStatusNotSupported), nil
	}
	return core.NewUnlockConnectorConfirmation(core.UnlockStatusUnlocked), nil
}

func (s *simulator) OnGetDiagnostics(r *firmware.GetDiagnosticsRequest) (*firmware.GetDiagnosticsConfirmation, error) {
	fmt.Printf("[REMOTE] GetDiagnostics location=%s\n", r.Location)
	conf := firmware.NewGetDiagnosticsConfirmation()
	conf.FileName = fmt.Sprintf("%s-diagnostics.log", s.clientID)
	go func() {
		time.Sleep(100 * time.Millisecond)
		_, _ = s.cp.DiagnosticsStatusNotification(firmware.DiagnosticsStatusUploading)
		time.Sleep(200 * time.Millisecond)
		_, _ = s.cp.DiagnosticsStatusNotification(firmware.DiagnosticsStatusUploaded)
	}()
	return conf, nil
}
func (s *simulator) OnUpdateFirmware(r *firmware.UpdateFirmwareRequest) (*firmware.UpdateFirmwareConfirmation, error) {
	fmt.Printf("[REMOTE] UpdateFirmware location=%s retrieveDate=%s\n", r.Location, r.RetrieveDate.String())
	go func() {
		time.Sleep(100 * time.Millisecond)
		for _, status := range []firmware.FirmwareStatus{firmware.FirmwareStatusDownloading, firmware.FirmwareStatusDownloaded, firmware.FirmwareStatusInstalling, firmware.FirmwareStatusInstalled} {
			_, _ = s.cp.FirmwareStatusNotification(status)
			time.Sleep(200 * time.Millisecond)
		}
	}()
	return firmware.NewUpdateFirmwareConfirmation(), nil
}
func (s *simulator) OnTriggerMessage(r *remotetrigger.TriggerMessageRequest) (*remotetrigger.TriggerMessageConfirmation, error) {
	fmt.Printf("[REMOTE] TriggerMessage requestedMessage=%s\n", r.RequestedMessage)
	implemented := r.RequestedMessage == core.BootNotificationFeatureName || r.RequestedMessage == core.HeartbeatFeatureName || r.RequestedMessage == core.StatusNotificationFeatureName || r.RequestedMessage == core.MeterValuesFeatureName
	if !implemented {
		return remotetrigger.NewTriggerMessageConfirmation(remotetrigger.TriggerMessageStatusNotImplemented), nil
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		var err error
		switch r.RequestedMessage {
		case core.BootNotificationFeatureName:
			err = s.boot()
		case core.HeartbeatFeatureName:
			err = s.heartbeat()
		case core.StatusNotificationFeatureName:
			for _, id := range s.connectorIDs() {
				c, _ := s.connector(id)
				c.mu.RLock()
				status, code := c.status, c.errorCode
				c.mu.RUnlock()
				if err = s.setStatus(c, status, code); err != nil {
					break
				}
			}
		case core.MeterValuesFeatureName:
			for _, id := range s.connectorIDs() {
				c, _ := s.connector(id)
				c.mu.RLock()
				active, power := c.transaction != 0, c.powerKW
				c.mu.RUnlock()
				if active {
					if err = s.sendMeter(c, 0, power); err != nil {
						break
					}
				}
			}
		}
		if err != nil {
			fmt.Printf("[SIM] triggered %s failed: %v\n", r.RequestedMessage, err)
		}
	}()
	return remotetrigger.NewTriggerMessageConfirmation(remotetrigger.TriggerMessageStatusAccepted), nil
}

func parseBoolPolicy(value, truthy, falsy string) (bool, bool) {
	if strings.EqualFold(value, truthy) {
		return true, true
	}
	if strings.EqualFold(value, falsy) {
		return false, true
	}
	return false, false
}
func normalizeWebSocketURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "ws" && parsed.Scheme != "wss") {
		return "", fmt.Errorf("OCPP URL must be an absolute ws:// or wss:// base URL: %q", value)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("OCPP base URL cannot contain a query string or fragment; the charger ID is appended as the final path segment")
	}
	return value, nil
}
func parseStatus(value string) (core.ChargePointStatus, bool) {
	for _, status := range []core.ChargePointStatus{core.ChargePointStatusAvailable, core.ChargePointStatusPreparing, core.ChargePointStatusCharging, core.ChargePointStatusSuspendedEVSE, core.ChargePointStatusSuspendedEV, core.ChargePointStatusFinishing, core.ChargePointStatusReserved, core.ChargePointStatusUnavailable, core.ChargePointStatusFaulted} {
		if strings.EqualFold(value, string(status)) {
			return status, true
		}
	}
	return "", false
}
func parseErrorCode(value string) (core.ChargePointErrorCode, bool) {
	for _, code := range []core.ChargePointErrorCode{core.ConnectorLockFailure, core.EVCommunicationError, core.GroundFailure, core.HighTemperature, core.InternalError, core.LocalListConflict, core.NoError, core.OtherError, core.OverCurrentFailure, core.OverVoltage, core.PowerMeterFailure, core.PowerSwitchFailure, core.ReaderFailure, core.ResetFailure, core.UnderVoltage, core.WeakSignal} {
		if strings.EqualFold(value, string(code)) {
			return code, true
		}
	}
	return "", false
}
func parseStopReason(value string) (core.Reason, bool) {
	for _, reason := range []core.Reason{core.ReasonDeAuthorized, core.ReasonEmergencyStop, core.ReasonEVDisconnected, core.ReasonHardReset, core.ReasonLocal, core.ReasonOther, core.ReasonPowerLoss, core.ReasonReboot, core.ReasonRemote, core.ReasonSoftReset, core.ReasonUnlockCommand} {
		if strings.EqualFold(value, string(reason)) {
			return reason, true
		}
	}
	return "", false
}
func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(env(key, strconv.Itoa(fallback)))
	if err != nil {
		return fallback
	}
	return value
}
func envFloat(key string, fallback float64) float64 {
	value, err := strconv.ParseFloat(env(key, strconv.FormatFloat(fallback, 'f', -1, 64)), 64)
	if err != nil {
		return fallback
	}
	return value
}
