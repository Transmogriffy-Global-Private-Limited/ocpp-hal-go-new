package store

import (
	"context"
	"sort"
	"time"
)

const v1TracePostStopAssociationWindow = 15 * time.Minute

// The memory implementation exists only for protocol tests. Production uses
// the PostgreSQL implementation below and retains evidence durably.
func (s *V1MemoryStore) EnsureV1Trace(_ context.Context, trace V1Trace) (*V1Trace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.traces[trace.TraceID]; existing != nil {
		return cloneV1Trace(existing), nil
	}
	if trace.CreatedAt.IsZero() {
		trace.CreatedAt = time.Now().UTC()
	}
	s.traces[trace.TraceID] = &trace
	return cloneV1Trace(&trace), nil
}
func (s *V1MemoryStore) BindV1TraceTransaction(_ context.Context, traceID string, tx *V1Transaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	trace := s.traces[traceID]
	if trace == nil {
		return ErrV1TransactionNotFound
	}
	trace.HALTransactionID = tx.HALTransactionID
	id := tx.OCPPTransactionID
	trace.OCPPTransactionID = &id
	return nil
}
func (s *V1MemoryStore) EnsureV1TraceForTransaction(ctx context.Context, tx *V1Transaction) (*V1Trace, error) {
	s.mu.Lock()
	for _, trace := range s.traces {
		if trace.HALTransactionID == tx.HALTransactionID {
			out := cloneV1Trace(trace)
			s.mu.Unlock()
			return out, nil
		}
		// CMS creates the root trace before RemoteStart. Once StartTransaction
		// supplies the authoritative OCPP transaction identity, bind that
		// existing connector-scoped root instead of creating a parallel trace.
		if tx.CMSStartIntentID != "" && trace.CMSStartIntentID == tx.CMSStartIntentID && trace.HALTransactionID == "" {
			trace.HALTransactionID = tx.HALTransactionID
			ocpp := tx.OCPPTransactionID
			trace.OCPPTransactionID = &ocpp
			out := cloneV1Trace(trace)
			s.mu.Unlock()
			return out, nil
		}
	}
	s.mu.Unlock()
	id, err := NewSecureUUIDString()
	if err != nil {
		return nil, err
	}
	ocpp := tx.OCPPTransactionID
	return s.EnsureV1Trace(ctx, V1Trace{TraceID: id, CPOID: tx.CPOID, CMSStartIntentID: tx.CMSStartIntentID, CMSCommandID: tx.CMSCommandID, HALTransactionID: tx.HALTransactionID, OCPPTransactionID: &ocpp, ChargerOCPPIdentity: tx.ChargerOCPPIdentity, OCPPConnectorNumber: tx.OCPPConnectorNumber})
}
func (s *V1MemoryStore) FindV1TraceByTransaction(_ context.Context, id string) (*V1Trace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, trace := range s.traces {
		if trace.HALTransactionID == id {
			return cloneV1Trace(trace), nil
		}
	}
	return nil, ErrV1TransactionNotFound
}
func (s *V1MemoryStore) FindV1TraceForConnector(_ context.Context, identity string, connector int) (*V1Trace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var candidate *V1Trace
	for _, trace := range s.traces {
		if trace.ChargerOCPPIdentity != identity || trace.OCPPConnectorNumber != connector {
			continue
		}
		transaction := s.transactions[trace.HALTransactionID]
		if trace.HALTransactionID != "" && transaction == nil {
			continue
		}
		if transaction != nil && transaction.CompletedAt != nil && transaction.CompletedAt.Before(time.Now().UTC().Add(-v1TracePostStopAssociationWindow)) {
			continue
		}
		if candidate == nil || traceAssociationPriority(transaction) < traceAssociationPriority(s.transactions[candidate.HALTransactionID]) || (traceAssociationPriority(transaction) == traceAssociationPriority(s.transactions[candidate.HALTransactionID]) && trace.CreatedAt.After(candidate.CreatedAt)) {
			candidate = trace
		}
	}
	if candidate == nil {
		return nil, ErrV1TransactionNotFound
	}
	return cloneV1Trace(candidate), nil
}

func traceAssociationPriority(transaction *V1Transaction) int {
	if transaction == nil {
		return 1 // A CMS RemoteStart root has not yet materialized.
	}
	if transaction.CompletedAt == nil {
		return 0
	}
	return 2
}
func (s *V1MemoryStore) AppendV1TraceEvent(_ context.Context, traceID string, input V1TraceEventInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.traces[traceID] == nil {
		return ErrV1TransactionNotFound
	}
	id, err := NewSecureUUIDString()
	if err != nil {
		return err
	}
	when := input.OccurredAt
	if when.IsZero() {
		when = time.Now().UTC()
	}
	s.traceEvents[traceID] = append(s.traceEvents[traceID], V1TraceEvent{EventID: id, TraceID: traceID, Source: input.Source, Target: input.Target, Category: input.Category, Protocol: input.Protocol, Phase: input.Phase, Summary: input.Summary, OccurredAt: when, RecordedAt: time.Now().UTC(), StateBefore: input.StateBefore, StateAfter: input.StateAfter, CorrelationID: input.CorrelationID, Data: sanitizeV1TraceData(input.Data)})
	return nil
}
func (s *V1MemoryStore) GetV1Trace(_ context.Context, id string) (*V1Trace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if trace := s.traces[id]; trace != nil {
		return cloneV1Trace(trace), nil
	}
	return nil, ErrV1TransactionNotFound
}
func (s *V1MemoryStore) ListV1TraceEvents(_ context.Context, id string, before time.Time, beforeID string, limit int) ([]V1TraceEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.traces[id] == nil {
		return nil, ErrV1TransactionNotFound
	}
	if !before.IsZero() && beforeID == "" {
		return nil, ErrV1InvalidEvidence
	}
	all := append([]V1TraceEvent(nil), s.traceEvents[id]...)
	sort.Slice(all, func(i, j int) bool {
		if all[i].OccurredAt.Equal(all[j].OccurredAt) {
			return all[i].EventID > all[j].EventID
		}
		return all[i].OccurredAt.After(all[j].OccurredAt)
	})
	out := make([]V1TraceEvent, 0, limit)
	for _, event := range all {
		if !before.IsZero() && (event.OccurredAt.After(before) || (event.OccurredAt.Equal(before) && event.EventID >= beforeID)) {
			continue
		}
		out = append(out, event)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (s *V1MemoryStore) DeleteV1TracesBefore(_ context.Context, before time.Time, limit int) (int64, error) {
	if limit < 1 {
		return 0, ErrV1InvalidEvidence
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, limit)
	for id, trace := range s.traces {
		if trace.CreatedAt.Before(before) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return s.traces[ids[i]].CreatedAt.Before(s.traces[ids[j]].CreatedAt) })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	for _, id := range ids {
		delete(s.traces, id)
		delete(s.traceEvents, id)
	}
	return int64(len(ids)), nil
}
func cloneV1Trace(value *V1Trace) *V1Trace {
	if value == nil {
		return nil
	}
	out := *value
	if value.OCPPTransactionID != nil {
		id := *value.OCPPTransactionID
		out.OCPPTransactionID = &id
	}
	return &out
}

var _ V1TraceStore = (*V1MemoryStore)(nil)
