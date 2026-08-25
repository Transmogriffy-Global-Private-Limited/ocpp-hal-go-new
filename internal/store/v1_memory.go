package store

import (
	"context"
	"sync"
	"time"
)

type V1MemoryStore struct {
	mu           sync.Mutex
	commands     map[string]*V1RemoteCommand
	credentials  map[string]*V1Credential
	transactions map[string]*V1Transaction
}

func NewV1MemoryStore() *V1MemoryStore {
	return &V1MemoryStore{
		commands:     make(map[string]*V1RemoteCommand),
		credentials:  make(map[string]*V1Credential),
		transactions: make(map[string]*V1Transaction),
	}
}

func (s *V1MemoryStore) CreateV1StartCommand(_ context.Context, input V1StartCommandInput) (*V1RemoteCommand, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.commands[input.CMSCommandID]; existing != nil {
		if existing.RequestDigest != input.RequestDigest {
			return nil, false, ErrV1IdempotencyConflict
		}
		return cloneV1Command(existing), true, nil
	}
	now := time.Now().UTC()
	halCommandID, err := NewSecureUUIDString()
	if err != nil {
		return nil, false, err
	}
	credentialExpiry := input.CredentialExpiresAt
	command := &V1RemoteCommand{HALCommandID: halCommandID, CMSCommandID: input.CMSCommandID, Kind: "START", RequestDigest: input.RequestDigest, CPOID: input.CPOID, CMSStartIntentID: input.CMSStartIntentID, CMSChargerID: input.CMSChargerID, CMSConnectorID: input.CMSConnectorID, ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: input.OCPPConnectorNumber, IDTag: input.IDTag, CredentialExpiresAt: &credentialExpiry, CommandExpiresAt: input.CommandExpiresAt, EnergyLimitWh: cloneInt64(input.EnergyLimitWh), MaxDurationSeconds: cloneInt64(input.MaxDurationSeconds), State: "PERSISTED", CreatedAt: now, UpdatedAt: now}
	s.commands[input.CMSCommandID] = command
	s.credentials[input.IDTag] = &V1Credential{IDTag: input.IDTag, CMSStartIntentID: input.CMSStartIntentID, CPOID: input.CPOID, CMSChargerID: input.CMSChargerID, CMSConnectorID: input.CMSConnectorID, ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: input.OCPPConnectorNumber, ExpiresAt: input.CredentialExpiresAt}
	return cloneV1Command(command), false, nil
}

func (s *V1MemoryStore) CreateV1StopCommand(_ context.Context, input V1StopCommandInput) (*V1RemoteCommand, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.commands[input.CMSCommandID]; existing != nil {
		if existing.RequestDigest != input.RequestDigest {
			return nil, false, ErrV1IdempotencyConflict
		}
		return cloneV1Command(existing), true, nil
	}
	now := time.Now().UTC()
	halCommandID, err := NewSecureUUIDString()
	if err != nil {
		return nil, false, err
	}
	txID := input.OCPPTransactionID
	command := &V1RemoteCommand{HALCommandID: halCommandID, CMSCommandID: input.CMSCommandID, Kind: "STOP", RequestDigest: input.RequestDigest, CPOID: input.CPOID, CMSChargingSessionID: input.CMSChargingSessionID, CMSChargerID: input.CMSChargerID, CMSConnectorID: input.CMSConnectorID, ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: input.OCPPConnectorNumber, HALTransactionID: input.HALTransactionID, OCPPTransactionID: &txID, RequestedStopInitiator: input.RequestedStopInitiator, RequestedStopReason: input.RequestedStopReason, CommandExpiresAt: input.CommandExpiresAt, State: "PERSISTED", CreatedAt: now, UpdatedAt: now}
	s.commands[input.CMSCommandID] = command
	return cloneV1Command(command), false, nil
}

func (s *V1MemoryStore) GetV1Command(_ context.Context, cmsCommandID string) (*V1RemoteCommand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if command := s.commands[cmsCommandID]; command != nil {
		return cloneV1Command(command), nil
	}
	return nil, ErrV1CommandNotFound
}
func (s *V1MemoryStore) GetV1Credential(_ context.Context, idTag string) (*V1Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if credential := s.credentials[idTag]; credential != nil {
		return cloneV1Credential(credential), nil
	}
	return nil, ErrV1CredentialRejected
}

func (s *V1MemoryStore) MaterializeV1Start(_ context.Context, input V1StartMaterialization) (*V1Transaction, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.MeterStartWh < 0 || input.OCPPTransactionID <= 0 || input.OCPPConnectorNumber <= 0 || !plausibleV1ProtocolTime(input.ActualStartedAt, input.ObservedAt) {
		return nil, false, ErrV1InvalidEvidence
	}
	credential := s.credentials[input.IDTag]
	if credential == nil || !credential.ExpiresAt.After(input.ObservedAt) || credential.ChargerOCPPIdentity != input.ChargerOCPPIdentity || (credential.OCPPConnectorNumber != 0 && credential.OCPPConnectorNumber != input.OCPPConnectorNumber) {
		return nil, false, ErrV1CredentialRejected
	}
	if credential.HALTransactionID != "" {
		tx := s.transactions[credential.HALTransactionID]
		if tx != nil && tx.MeterStartWh == input.MeterStartWh {
			return cloneV1Transaction(tx), true, nil
		}
		return nil, false, ErrV1CredentialRejected
	}
	var command *V1RemoteCommand
	for _, candidate := range s.commands {
		if candidate.Kind == "START" && candidate.IDTag == input.IDTag {
			command = candidate
			break
		}
	}
	if command == nil || !command.CommandExpiresAt.After(input.ObservedAt) {
		return nil, false, ErrV1CredentialRejected
	}
	halTransactionID, err := NewSecureUUIDString()
	if err != nil {
		return nil, false, err
	}
	tx := &V1Transaction{HALTransactionID: halTransactionID, CMSStartIntentID: credential.CMSStartIntentID, CMSCommandID: command.CMSCommandID, CPOID: credential.CPOID, CMSChargerID: credential.CMSChargerID, CMSConnectorID: credential.CMSConnectorID, ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: input.OCPPConnectorNumber, IDTag: input.IDTag, OCPPTransactionID: input.OCPPTransactionID, ActualStartedAt: input.ActualStartedAt, ObservedStartedAt: input.ObservedAt, MeterStartWh: input.MeterStartWh, EnergyLimitWh: cloneInt64(command.EnergyLimitWh), MaxDurationSeconds: cloneInt64(command.MaxDurationSeconds), StopState: "NONE"}
	if tx.MaxDurationSeconds != nil {
		deadline := input.ObservedAt.Add(time.Duration(*tx.MaxDurationSeconds) * time.Second)
		tx.StopDeadlineAt = &deadline
	}
	credential.HALTransactionID = halTransactionID
	consumed := input.ObservedAt
	credential.ConsumedAt = &consumed
	command.HALTransactionID = halTransactionID
	command.OCPPTransactionID = &input.OCPPTransactionID
	command.State = "MATERIALIZED"
	command.UpdatedAt = input.ObservedAt
	s.transactions[halTransactionID] = tx
	return cloneV1Transaction(tx), false, nil
}

func (s *V1MemoryStore) UpdateV1Meter(_ context.Context, halTransactionID string, ocppTransactionID int64, meterWh int64, observedAt time.Time) (*V1Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx := s.transactions[halTransactionID]
	if tx == nil || tx.OCPPTransactionID != ocppTransactionID || tx.CompletedAt != nil || meterWh < tx.MeterStartWh {
		return nil, ErrV1TransactionNotFound
	}
	if tx.LatestMeterWh != nil && meterWh < *tx.LatestMeterWh {
		classification, err := classifyV1MeterEvidence(tx.MeterStartWh, tx.LatestMeterWh, meterWh)
		if err != nil || classification.Class != v1MeterEvidenceQuantizationNormalized {
			return cloneV1Transaction(tx), ErrV1InvalidEvidence
		}
		tx.MeterQuantizationAnomalyCount++
		return cloneV1Transaction(tx), nil
	}
	tx.LatestMeterWh = &meterWh
	consumed := meterWh - tx.MeterStartWh
	tx.ConsumedWh = &consumed
	if tx.MeterObservedAt == nil || observedAt.After(*tx.MeterObservedAt) {
		tx.MeterObservedAt = &observedAt
	}
	tx.MeterSequence++
	return cloneV1Transaction(tx), nil
}

func (s *V1MemoryStore) UpdateV1TelemetryForOCPP(_ context.Context, identity string, ocppTransactionID int64, telemetry V1MeterTelemetry) (*V1Transaction, V1TelemetryUpdateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := V1TelemetryUpdateResult{}
	for _, transaction := range s.transactions {
		if transaction.ChargerOCPPIdentity != identity || transaction.OCPPTransactionID != ocppTransactionID {
			continue
		}
		if transaction.CompletedAt != nil {
			return cloneV1Transaction(transaction), result, nil
		}
		if telemetry.EnergyWh != nil && telemetry.EnergyObservedAt != nil && *telemetry.EnergyWh >= transaction.MeterStartWh && (transaction.LatestMeterWh == nil || *telemetry.EnergyWh >= *transaction.LatestMeterWh) {
			meter := *telemetry.EnergyWh
			transaction.LatestMeterWh = &meter
			consumed := meter - transaction.MeterStartWh
			transaction.ConsumedWh = &consumed
			observed := *telemetry.EnergyObservedAt
			if transaction.MeterObservedAt != nil && observed.Before(*transaction.MeterObservedAt) {
				observed = *transaction.MeterObservedAt
			}
			transaction.MeterObservedAt = &observed
			transaction.MeterSequence++
			result.EnergyAccepted = true
		}
		if telemetry.SoCPercent != nil && telemetry.SoCObservedAt != nil {
			observed, soc := *telemetry.SoCObservedAt, *telemetry.SoCPercent
			if transaction.SoCObservedAt == nil || observed.After(*transaction.SoCObservedAt) {
				if transaction.InitialSoCPercent == nil {
					initial := soc
					transaction.InitialSoCPercent = &initial
				}
				transaction.LatestSoCPercent, transaction.SoCObservedAt = &soc, &observed
				transaction.SoCSequence++
				result.SoCAccepted = true
			}
		}
		return cloneV1Transaction(transaction), result, nil
	}
	return nil, result, ErrV1TransactionNotFound
}
func (s *V1MemoryStore) RequestV1Stop(_ context.Context, halTransactionID, initiator, reason string) (*V1Transaction, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx := s.transactions[halTransactionID]
	if tx == nil || tx.CompletedAt != nil {
		return nil, false, ErrV1TransactionNotFound
	}
	if tx.StopState != "NONE" {
		return cloneV1Transaction(tx), false, nil
	}
	tx.StopState = "PENDING_DELIVERY"
	tx.RequestedStopInitiator = initiator
	tx.RequestedStopReason = reason
	return cloneV1Transaction(tx), true, nil
}
func (s *V1MemoryStore) CompleteV1Transaction(_ context.Context, halTransactionID string, meterStopWh int64, ocppReason string, completedAt, observedAt time.Time) (*V1Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx := s.transactions[halTransactionID]
	if tx == nil {
		return nil, ErrV1TransactionNotFound
	}
	if tx.CompletedAt != nil {
		expectedRaw := tx.MeterStopWh
		if tx.RawMeterStopWh != nil {
			expectedRaw = tx.RawMeterStopWh
		}
		if expectedRaw == nil || *expectedRaw != meterStopWh || tx.OCPPStopReason != ocppReason || !tx.CompletedAt.Equal(completedAt) {
			return nil, ErrV1InvalidEvidence
		}
		return cloneV1Transaction(tx), nil
	}
	if completedAt.Before(tx.ActualStartedAt) || observedAt.Before(tx.ObservedStartedAt) || !plausibleV1ProtocolTime(completedAt, observedAt) {
		return nil, ErrV1InvalidEvidence
	}
	if tx.MeterObservedAt != nil && tx.MeterObservedAt.After(completedAt) && tx.LatestMeterWh != nil && meterStopWh < *tx.LatestMeterWh {
		return nil, ErrV1InvalidEvidence
	}
	classification, err := classifyV1MeterEvidence(tx.MeterStartWh, tx.LatestMeterWh, meterStopWh)
	if err != nil {
		return nil, ErrV1InvalidEvidence
	}
	effectiveStopWh := classification.EffectiveWh
	tx.MeterStopWh = &effectiveStopWh
	tx.RawMeterStopWh = &meterStopWh
	adjustment := classification.AdjustmentWh
	tx.MeterStopAdjustmentWh = &adjustment
	tx.MeterStopEvidence = string(classification.Class)
	tx.LatestMeterWh = &effectiveStopWh
	consumed := effectiveStopWh - tx.MeterStartWh
	tx.ConsumedWh = &consumed
	if tx.MeterObservedAt == nil || completedAt.After(*tx.MeterObservedAt) {
		tx.MeterObservedAt = &completedAt
	}
	tx.OCPPStopReason = ocppReason
	tx.StopState = "COMPLETED"
	tx.CompletedAt = &completedAt
	tx.ObservedCompletedAt = &observedAt
	return cloneV1Transaction(tx), nil
}
func (s *V1MemoryStore) GetV1Transaction(_ context.Context, id string) (*V1Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tx := s.transactions[id]; tx != nil {
		return cloneV1Transaction(tx), nil
	}
	return nil, ErrV1TransactionNotFound
}

// V1MemoryStore intentionally has no durable fact outbox. It remains a test
// helper and cannot claim to recover a terminal production delivery state.
func (s *V1MemoryStore) RequeueV1Fact(context.Context, string, string) error {
	return ErrV1FactNotFound
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func cloneV1Command(command *V1RemoteCommand) *V1RemoteCommand {
	if command == nil {
		return nil
	}
	copy := *command
	copy.EnergyLimitWh = cloneInt64(command.EnergyLimitWh)
	copy.MaxDurationSeconds = cloneInt64(command.MaxDurationSeconds)
	if command.CredentialExpiresAt != nil {
		value := *command.CredentialExpiresAt
		copy.CredentialExpiresAt = &value
	}
	if command.OCPPTransactionID != nil {
		value := *command.OCPPTransactionID
		copy.OCPPTransactionID = &value
	}
	return &copy
}
func cloneV1Credential(credential *V1Credential) *V1Credential {
	if credential == nil {
		return nil
	}
	copy := *credential
	if credential.ConsumedAt != nil {
		value := *credential.ConsumedAt
		copy.ConsumedAt = &value
	}
	return &copy
}
func cloneV1Transaction(transaction *V1Transaction) *V1Transaction {
	if transaction == nil {
		return nil
	}
	copy := *transaction
	copy.LatestMeterWh = cloneInt64(transaction.LatestMeterWh)
	copy.ConsumedWh = cloneInt64(transaction.ConsumedWh)
	copy.MeterStopWh = cloneInt64(transaction.MeterStopWh)
	copy.RawMeterStopWh = cloneInt64(transaction.RawMeterStopWh)
	copy.MeterStopAdjustmentWh = cloneInt64(transaction.MeterStopAdjustmentWh)
	copy.InitialSoCPercent = cloneV1SoCPercent(transaction.InitialSoCPercent)
	copy.LatestSoCPercent = cloneV1SoCPercent(transaction.LatestSoCPercent)
	copy.EnergyLimitWh = cloneInt64(transaction.EnergyLimitWh)
	copy.MaxDurationSeconds = cloneInt64(transaction.MaxDurationSeconds)
	if transaction.MeterObservedAt != nil {
		value := *transaction.MeterObservedAt
		copy.MeterObservedAt = &value
	}
	if transaction.SoCObservedAt != nil {
		value := *transaction.SoCObservedAt
		copy.SoCObservedAt = &value
	}
	if transaction.StopDeadlineAt != nil {
		value := *transaction.StopDeadlineAt
		copy.StopDeadlineAt = &value
	}
	if transaction.CompletedAt != nil {
		value := *transaction.CompletedAt
		copy.CompletedAt = &value
	}
	if transaction.ObservedCompletedAt != nil {
		value := *transaction.ObservedCompletedAt
		copy.ObservedCompletedAt = &value
	}
	return &copy
}

func cloneV1SoCPercent(value *V1SoCPercent) *V1SoCPercent {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
