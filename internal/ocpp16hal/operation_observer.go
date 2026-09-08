package ocpp16hal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp"
	ocpp16 "github.com/lorenzodonini/ocpp-go/ocpp1.6"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/certificates"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/extendedtriggermessage"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/firmware"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/localauth"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/logging"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/remotetrigger"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/reservation"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/securefirmware"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/security"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/smartcharging"
	"github.com/lorenzodonini/ocpp-go/ocppj"
	"github.com/lorenzodonini/ocpp-go/ws"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

// operationObserver is intentionally not a packet logger. It registers only
// durable CPO operations, binds them to the library-created CALL unique ID,
// and persists only the action-specific public-safe evidence.
type operationObserver struct {
	traces    store.V1TraceStore
	mu        sync.Mutex
	byRequest map[uintptr]*observedOperation
	byUnique  map[string]*observedOperation
}
type observedOperation struct {
	traceID, cmsOperationID, halOperationID, chargerID, action, uniqueID string
	connector                                                            int
	sentPayload                                                          map[string]any
}

func newOperationObserver(traces store.V1TraceStore) *operationObserver {
	return &operationObserver{traces: traces, byRequest: map[uintptr]*observedOperation{}, byUnique: map[string]*observedOperation{}}
}
func newObservedCentralSystem(observer *operationObserver) ocpp16.CentralSystem {
	transport := &observedWsServer{WsServer: ws.NewServer(), observer: observer}
	dispatcher := &observedDispatcher{ServerDispatcher: ocppj.NewDefaultServerDispatcher(ocppj.NewFIFOQueueMap(0)), observer: observer}
	endpoint := ocppj.NewServer(transport, dispatcher, nil, core.Profile, localauth.Profile, firmware.Profile, reservation.Profile, remotetrigger.Profile, smartcharging.Profile, logging.Profile, security.Profile, extendedtriggermessage.Profile, certificates.Profile, securefirmware.Profile)
	return ocpp16.NewCentralSystem(endpoint, transport)
}
func requestKey(request ocpp.Request) uintptr {
	// All current OCPP requests are pointers. A non-pointer is not eligible for
	// audited operation registration, rather than risking a cross-operation bind.
	v := reflect.ValueOf(request)
	if !v.IsValid() || v.Kind() != reflect.Ptr || v.IsNil() {
		return 0
	}
	return v.Pointer()
}
func (o *operationObserver) register(request ocpp.Request, operation *store.V1ChargerOperation) {
	if o == nil || o.traces == nil || operation == nil || operation.TraceID == "" {
		return
	}
	key := requestKey(request)
	if key == 0 {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.byRequest[key] = &observedOperation{traceID: operation.TraceID, cmsOperationID: operation.CMSOperationID, halOperationID: operation.HALOperationID, chargerID: operation.ChargerOCPPIdentity, action: operationAction(operation.Kind), connector: operation.OCPPConnectorNumber, sentPayload: safeOperationCall(operation)}
}
func (o *operationObserver) bind(request ocpp.Request, uniqueID string) {
	if o == nil || uniqueID == "" {
		return
	}
	key := requestKey(request)
	if key == 0 {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if entry := o.byRequest[key]; entry != nil {
		delete(o.byRequest, key)
		entry.uniqueID = uniqueID
		o.byUnique[uniqueID] = entry
	}
}
func (o *operationObserver) sent(uniqueID string) {
	entry := o.lookup(uniqueID)
	if entry == nil {
		return
	}
	o.append(entry, "CALL", entry.sentPayload)
}
func (o *operationObserver) received(uniqueID, messageType string, payload map[string]any) {
	entry := o.lookup(uniqueID)
	if entry == nil {
		return
	}
	o.append(entry, messageType, safeOperationResponse(entry.action, messageType, payload))
	o.mu.Lock()
	delete(o.byUnique, uniqueID)
	o.mu.Unlock()
}
func (o *operationObserver) forget(uniqueID string) {
	o.mu.Lock()
	delete(o.byUnique, uniqueID)
	o.mu.Unlock()
}
func (o *operationObserver) forgetTrace(traceID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for key, entry := range o.byRequest {
		if entry.traceID == traceID {
			delete(o.byRequest, key)
		}
	}
	for key, entry := range o.byUnique {
		if entry.traceID == traceID {
			delete(o.byUnique, key)
		}
	}
}
func (o *operationObserver) forgetCharger(chargerID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for id, entry := range o.byUnique {
		if entry.chargerID == chargerID {
			delete(o.byUnique, id)
		}
	}
}
func (o *operationObserver) lookup(uniqueID string) *observedOperation {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.byUnique[uniqueID]
}
func (o *operationObserver) append(entry *observedOperation, messageType string, payload map[string]any) {
	if o.traces == nil {
		return
	}
	_ = o.traces.AppendV1TraceEvent(context.Background(), entry.traceID, store.V1TraceEventInput{Source: "HAL", Target: "CMS", Category: "CHARGER_OPERATION_OCPP", Protocol: "OCPP1.6", Phase: "STARTING", Summary: "CPO operation OCPP " + messageType, OccurredAt: time.Now().UTC(), Data: map[string]any{"unique_id": entryUnique(entry, messageType), "action": entry.action, "message_type": messageType, "direction": operationDirection(messageType), "payload": payload}})
}

// unique identity is installed by append caller below through this transient map-free field.
func entryUnique(entry *observedOperation, _ string) string { return entry.uniqueID }
func operationDirection(messageType string) string {
	if messageType == "CALL" {
		return "OUTBOUND"
	}
	return "INBOUND"
}

// observedWsServer keeps normal ocpp-go behavior intact. Write returning nil is
// the strongest send boundary exposed by the pinned library: the library has
// accepted the CALL at its websocket write boundary (its transport then owns
// the asynchronous socket flush).
type observedWsServer struct {
	ws.WsServer
	observer *operationObserver
}

func (s *observedWsServer) Write(id string, data []byte) error {
	err := s.WsServer.Write(id, data)
	if err == nil {
		if unique, typ, _, ok := parseOCPPFrame(data); ok && typ == "CALL" {
			s.observer.sent(unique)
		}
	}
	return err
}
func (s *observedWsServer) SetMessageHandler(handler func(ws.Channel, []byte) error) {
	s.WsServer.SetMessageHandler(func(channel ws.Channel, data []byte) error {
		if unique, typ, payload, ok := parseOCPPFrame(data); ok && (typ == "CALLRESULT" || typ == "CALLERROR") {
			s.observer.received(unique, typ, payload)
		}
		return handler(channel, data)
	})
}

type observedDispatcher struct {
	ocppj.ServerDispatcher
	observer *operationObserver
}

func (d *observedDispatcher) SendRequest(clientID string, bundle ocppj.RequestBundle) error {
	d.observer.bind(bundle.Call.Payload, bundle.Call.UniqueId)
	if err := d.ServerDispatcher.SendRequest(clientID, bundle); err != nil {
		d.observer.forget(bundle.Call.UniqueId)
		return err
	}
	return nil
}

func parseOCPPFrame(raw []byte) (string, string, map[string]any, bool) {
	var frame []json.RawMessage
	if json.Unmarshal(raw, &frame) != nil || len(frame) < 3 {
		return "", "", nil, false
	}
	var kind int
	var unique string
	if json.Unmarshal(frame[0], &kind) != nil || json.Unmarshal(frame[1], &unique) != nil {
		return "", "", nil, false
	}
	if kind == 2 && len(frame) == 4 {
		return unique, "CALL", nil, true
	}
	if kind == 3 && len(frame) == 3 {
		var payload map[string]any
		if json.Unmarshal(frame[2], &payload) != nil {
			return "", "", nil, false
		}
		return unique, "CALLRESULT", payload, true
	}
	if kind == 4 && len(frame) == 5 {
		var code string
		if json.Unmarshal(frame[2], &code) != nil {
			return "", "", nil, false
		}
		return unique, "CALLERROR", map[string]any{"error_code": code}, true
	}
	return "", "", nil, false
}
func operationAction(kind string) string {
	return map[string]string{"RESET": "Reset", "UNLOCK_CONNECTOR": "UnlockConnector", "CHANGE_AVAILABILITY": "ChangeAvailability", "CLEAR_CACHE": "ClearCache", "CHANGE_CONFIGURATION": "ChangeConfiguration", "TRIGGER_MESSAGE": "TriggerMessage", "GET_CONFIGURATION": "GetConfiguration"}[kind]
}
func safeOperationCall(op *store.V1ChargerOperation) map[string]any {
	switch op.Kind {
	case "RESET":
		return map[string]any{"type": strings.Title(strings.ToLower(op.Parameters["type"]))}
	case "UNLOCK_CONNECTOR":
		return map[string]any{"connectorId": op.OCPPConnectorNumber}
	case "CHANGE_AVAILABILITY":
		return map[string]any{"connectorId": op.OCPPConnectorNumber, "type": strings.Title(strings.ToLower(op.Parameters["type"]))}
	case "CLEAR_CACHE":
		return map[string]any{}
	case "CHANGE_CONFIGURATION":
		return map[string]any{"key": op.Parameters["key"], "redacted": true}
	case "TRIGGER_MESSAGE":
		p := map[string]any{"requestedMessage": op.Parameters["requested_message"]}
		if op.OCPPConnectorNumber > 0 {
			p["connectorId"] = op.OCPPConnectorNumber
		}
		return p
	case "GET_CONFIGURATION":
		keys := make([]any, 0, len(op.ConfigurationKeys))
		for _, key := range op.ConfigurationKeys {
			keys = append(keys, key)
		}
		return map[string]any{"configuration_keys": keys}
	}
	return map[string]any{}
}
func safeOperationResponse(action, messageType string, payload map[string]any) map[string]any {
	if messageType == "CALLERROR" {
		return map[string]any{"error_code": payload["error_code"]}
	}
	if action == "GetConfiguration" {
		keys, _ := payload["configurationKey"].([]any)
		safe := make([]any, 0, len(keys))
		for _, raw := range keys {
			item, _ := raw.(map[string]any)
			key, _ := item["key"].(string)
			v := map[string]any{"key": key}
			if sensitiveConfigurationKey(key) {
				v["redacted"] = true
			} else {
				v["value"] = item["value"]
				v["redacted"] = false
			}
			safe = append(safe, v)
		}
		unknown, _ := payload["unknownKey"].([]any)
		return map[string]any{"configuration_keys": safe, "unknown_keys": unknown}
	}
	return map[string]any{"status": payload["status"]}
}
func sensitiveConfigurationKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "password") || strings.Contains(key, "secret") || strings.Contains(key, "token") || strings.Contains(key, "privatekey") || strings.Contains(key, "certificate") || key == "authorizationkey"
}

// DispatchChargerOperation uses the same typed OCPP requests as the existing
// helpers, but registers this durable operation before ocpp-go creates its
// unique ID. This is the only path that produces CPO-operation evidence.
func (h *HAL) DispatchChargerOperation(ctx context.Context, operation *store.V1ChargerOperation) (string, *core.GetConfigurationConfirmation, error) {
	if h == nil || operation == nil {
		return "", nil, errors.New("charger operation is required")
	}
	var request ocpp.Request
	switch operation.Kind {
	case "RESET":
		if operation.Parameters["type"] == "SOFT" {
			request = core.NewResetRequest(core.ResetTypeSoft)
		} else {
			request = core.NewResetRequest(core.ResetTypeHard)
		}
	case "UNLOCK_CONNECTOR":
		request = core.NewUnlockConnectorRequest(operation.OCPPConnectorNumber)
	case "CHANGE_AVAILABILITY":
		kind := core.AvailabilityTypeOperative
		if operation.Parameters["type"] == "INOPERATIVE" {
			kind = core.AvailabilityTypeInoperative
		}
		request = core.NewChangeAvailabilityRequest(operation.OCPPConnectorNumber, kind)
	case "CLEAR_CACHE":
		request = core.NewClearCacheRequest()
	case "CHANGE_CONFIGURATION":
		request = core.NewChangeConfigurationRequest(operation.Parameters["key"], operation.Parameters["value"])
	case "TRIGGER_MESSAGE":
		req := remotetrigger.NewTriggerMessageRequest(remotetrigger.MessageTrigger(operation.Parameters["requested_message"]))
		if operation.OCPPConnectorNumber > 0 {
			connector := operation.OCPPConnectorNumber
			req.ConnectorId = &connector
		}
		request = req
	case "GET_CONFIGURATION":
		request = core.NewGetConfigurationRequest(operation.ConfigurationKeys)
	default:
		return "", nil, fmt.Errorf("unsupported charger operation %q", operation.Kind)
	}
	chargerID := h.wireIdentityFor(operation.ChargerOCPPIdentity)
	h.operationObserver.register(request, operation)
	result := make(chan struct {
		status        string
		configuration *core.GetConfigurationConfirmation
		err           error
	}, 1)
	err := h.cs.SendRequestAsync(chargerID, request, func(response ocpp.Response, callbackErr error) {
		if callbackErr != nil {
			result <- struct {
				status        string
				configuration *core.GetConfigurationConfirmation
				err           error
			}{"", nil, callbackErr}
			return
		}
		if response == nil {
			result <- struct {
				status        string
				configuration *core.GetConfigurationConfirmation
				err           error
			}{"", nil, errors.New("nil charger operation confirmation")}
			return
		}
		var view map[string]any
		raw, _ := json.Marshal(response)
		_ = json.Unmarshal(raw, &view)
		status, _ := view["status"].(string)
		configuration, _ := response.(*core.GetConfigurationConfirmation)
		result <- struct {
			status        string
			configuration *core.GetConfigurationConfirmation
			err           error
		}{status, configuration, nil}
	})
	if err != nil {
		return "", nil, err
	}
	select {
	case outcome := <-result:
		return outcome.status, outcome.configuration, outcome.err
	case <-ctx.Done():
		h.operationObserver.forgetTrace(operation.TraceID)
		return "", nil, ctx.Err()
	}
}
