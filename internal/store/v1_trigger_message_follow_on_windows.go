package store

import (
	"context"
	"sort"
	"time"
)

const v1TriggerMessageFollowOnWindow = time.Minute

func (s *V1MemoryStore) OpenV1TriggerMessageFollowOnWindow(_ context.Context, traceID, requestedMessage string, acceptedAt time.Time) error {
	if !V1TriggerMessageAction(requestedMessage) || acceptedAt.IsZero() {
		return ErrV1InvalidEvidence
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	trace := s.traces[traceID]
	if trace == nil {
		return ErrV1TransactionNotFound
	}
	if existing := s.followOnWindows[traceID]; existing != nil {
		return nil
	}
	acceptedAt = acceptedAt.UTC()
	s.followOnWindows[traceID] = &V1TriggerMessageFollowOnWindow{TraceID: traceID, RequestedMessage: requestedMessage, ChargerOCPPIdentity: trace.ChargerOCPPIdentity, OCPPConnectorNumber: trace.OCPPConnectorNumber, AcceptedAt: acceptedAt, DeadlineAt: acceptedAt.Add(v1TriggerMessageFollowOnWindow), State: "OPEN"}
	return nil
}

func (s *V1MemoryStore) RecordV1TriggerMessageFollowOn(_ context.Context, identity, action string, connector int, observedAt time.Time) (int, error) {
	if !V1TriggerMessageAction(action) || (V1TriggerMessageConnectorScoped(action) && connector < 1) || observedAt.IsZero() {
		return 0, ErrV1InvalidEvidence
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0)
	for traceID, window := range s.followOnWindows {
		if window.RequestedMessage != action || window.ChargerOCPPIdentity != identity || window.State == "OBSERVED" || !window.AcceptedAt.Before(observedAt) || window.DeadlineAt.Before(observedAt) {
			continue
		}
		if V1TriggerMessageConnectorScoped(action) && window.OCPPConnectorNumber > 0 && window.OCPPConnectorNumber != connector {
			continue
		}
		ids = append(ids, traceID)
	}
	sort.Strings(ids)
	for _, traceID := range ids {
		window := s.followOnWindows[traceID]
		if err := s.appendV1FollowOnEventLocked(traceID, V1TraceEventInput{Source: "CHARGER", Target: "HAL", Category: "CHARGER_OPERATION_FOLLOW_ON", Protocol: "OCPP1.6", Phase: "CHARGING", Summary: "TriggerMessage follow-on observed", OccurredAt: observedAt, Data: v1TriggerMessageFollowOnData(window, action, connector)}); err != nil {
			return 0, err
		}
		window.State = "OBSERVED"
	}
	return len(ids), nil
}

func (s *V1MemoryStore) CloseV1TriggerMessageFollowOnWindows(_ context.Context, now time.Time, limit int) (int, error) {
	if limit < 1 {
		return 0, ErrV1InvalidEvidence
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, limit)
	for traceID, window := range s.followOnWindows {
		if window.State == "OPEN" && !window.DeadlineAt.After(now) {
			ids = append(ids, traceID)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return s.followOnWindows[ids[i]].DeadlineAt.Before(s.followOnWindows[ids[j]].DeadlineAt)
	})
	if len(ids) > limit {
		ids = ids[:limit]
	}
	for _, traceID := range ids {
		window := s.followOnWindows[traceID]
		if err := s.appendV1FollowOnEventLocked(traceID, V1TraceEventInput{Source: "HAL", Target: "CMS", Category: "CHARGER_OPERATION_FOLLOW_ON_CLOSED", Protocol: "OCPP1.6", Phase: "CHARGING", Summary: "TriggerMessage follow-on window closed", OccurredAt: now, Data: v1TriggerMessageFollowOnClosureData(window)}); err != nil {
			return 0, err
		}
		window.State = "CLOSED"
	}
	return len(ids), nil
}

func (s *V1MemoryStore) appendV1FollowOnEventLocked(traceID string, input V1TraceEventInput) error {
	if s.traces[traceID] == nil {
		return ErrV1TransactionNotFound
	}
	id, err := NewSecureUUIDString()
	if err != nil {
		return err
	}
	when := input.OccurredAt.UTC()
	s.traceEvents[traceID] = append(s.traceEvents[traceID], V1TraceEvent{EventID: id, TraceID: traceID, Source: input.Source, Target: input.Target, Category: input.Category, Protocol: input.Protocol, Phase: input.Phase, Summary: input.Summary, OccurredAt: when, RecordedAt: time.Now().UTC(), Data: sanitizeV1TraceData(input.Data)})
	return nil
}

func v1TriggerMessageFollowOnData(window *V1TriggerMessageFollowOnWindow, action string, connector int) map[string]any {
	data := map[string]any{"follow_on": true, "expected_message": window.RequestedMessage, "observed_action": action, "charger_ocpp_identity": window.ChargerOCPPIdentity}
	if V1TriggerMessageConnectorScoped(action) {
		data["connector_number"] = connector
	}
	return data
}

func v1TriggerMessageFollowOnClosureData(window *V1TriggerMessageFollowOnWindow) map[string]any {
	data := map[string]any{"follow_on_closed": true, "expected_message": window.RequestedMessage, "charger_ocpp_identity": window.ChargerOCPPIdentity, "accepted_at": window.AcceptedAt.UTC().Format(time.RFC3339Nano)}
	if V1TriggerMessageConnectorScoped(window.RequestedMessage) {
		data["connector_number"] = window.OCPPConnectorNumber
	}
	return data
}

var _ V1TriggerMessageFollowOnWindowStore = (*V1MemoryStore)(nil)
