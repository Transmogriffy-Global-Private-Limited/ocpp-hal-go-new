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
	credentialExpiry := input.CredentialExpiresAt
	command := &V1RemoteCommand{HALCommandID: NewUUIDString(), CMSCommandID: input.CMSCommandID, Kind: "START", RequestDigest: input.RequestDigest, CPOID: input.CPOID, CMSStartIntentID: input.CMSStartIntentID, CMSChargerID: input.CMSChargerID, CMSConnectorID: input.CMSConnectorID, ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: input.OCPPConnectorNumber, IDTag: input.IDTag, CredentialExpiresAt: &credentialExpiry, CommandExpiresAt: input.CommandExpiresAt, EnergyLimitWh: cloneInt64(input.EnergyLimitWh), MaxDurationSeconds: cloneInt64(input.MaxDurationSeconds), State: "PERSISTED", CreatedAt: now, UpdatedAt: now}
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
	txID := input.OCPPTransactionID
	command := &V1RemoteCommand{HALCommandID: NewUUIDString(), CMSCommandID: input.CMSCommandID, Kind: "STOP", RequestDigest: input.RequestDigest, CPOID: input.CPOID, CMSChargingSessionID: input.CMSChargingSessionID, CMSChargerID: input.CMSChargerID, CMSConnectorID: input.CMSConnectorID, ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: input.OCPPConnectorNumber, HALTransactionID: input.HALTransactionID, OCPPTransactionID: &txID, RequestedStopInitiator: input.RequestedStopInitiator, RequestedStopReason: input.RequestedStopReason, CommandExpiresAt: input.CommandExpiresAt, State: "PERSISTED", CreatedAt: now, UpdatedAt: now}
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
	credential := s.credentials[input.IDTag]
	if credential == nil || !credential.ExpiresAt.After(input.ActualStartedAt) || credential.ChargerOCPPIdentity != input.ChargerOCPPIdentity || (credential.OCPPConnectorNumber != 0 && credential.OCPPConnectorNumber != input.OCPPConnectorNumber) {
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
	if command == nil || !command.CommandExpiresAt.After(input.ActualStartedAt) {
		return nil, false, ErrV1CredentialRejected
	}
	halTransactionID := NewUUIDString()
	tx := &V1Transaction{HALTransactionID: halTransactionID, CMSStartIntentID: credential.CMSStartIntentID, CMSCommandID: command.CMSCommandID, CPOID: credential.CPOID, CMSChargerID: credential.CMSChargerID, CMSConnectorID: credential.CMSConnectorID, ChargerOCPPIdentity: input.ChargerOCPPIdentity, OCPPConnectorNumber: input.OCPPConnectorNumber, IDTag: input.IDTag, OCPPTransactionID: input.OCPPTransactionID, ActualStartedAt: input.ActualStartedAt, MeterStartWh: input.MeterStartWh, EnergyLimitWh: cloneInt64(command.EnergyLimitWh), MaxDurationSeconds: cloneInt64(command.MaxDurationSeconds), StopState: "NONE"}
	if tx.MaxDurationSeconds != nil {
		deadline := input.ActualStartedAt.Add(time.Duration(*tx.MaxDurationSeconds) * time.Second)
		tx.StopDeadlineAt = &deadline
	}
	credential.HALTransactionID = halTransactionID
	consumed := input.ActualStartedAt
	credential.ConsumedAt = &consumed
	command.HALTransactionID = halTransactionID
	command.OCPPTransactionID = &input.OCPPTransactionID
	command.State = "MATERIALIZED"
	command.UpdatedAt = input.ActualStartedAt
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
	tx.LatestMeterWh = &meterWh
	consumed := meterWh - tx.MeterStartWh
	tx.ConsumedWh = &consumed
	tx.MeterObservedAt = &observedAt
	tx.MeterSequence++
	return cloneV1Transaction(tx), nil
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
func (s *V1MemoryStore) CompleteV1Transaction(_ context.Context, halTransactionID string, meterStopWh int64, ocppReason string, completedAt time.Time) (*V1Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx := s.transactions[halTransactionID]
	if tx == nil {
		return nil, ErrV1TransactionNotFound
	}
	if tx.CompletedAt != nil {
		return cloneV1Transaction(tx), nil
	}
	tx.MeterStopWh = &meterStopWh
	tx.LatestMeterWh = &meterStopWh
	if meterStopWh >= tx.MeterStartWh {
		consumed := meterStopWh - tx.MeterStartWh
		tx.ConsumedWh = &consumed
	} else {
		tx.ConsumedWh = nil
	}
	tx.MeterObservedAt = &completedAt
	tx.OCPPStopReason = ocppReason
	tx.StopState = "COMPLETED"
	tx.CompletedAt = &completedAt
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
	copy.EnergyLimitWh = cloneInt64(transaction.EnergyLimitWh)
	copy.MaxDurationSeconds = cloneInt64(transaction.MaxDurationSeconds)
	if transaction.MeterObservedAt != nil {
		value := *transaction.MeterObservedAt
		copy.MeterObservedAt = &value
	}
	if transaction.StopDeadlineAt != nil {
		value := *transaction.StopDeadlineAt
		copy.StopDeadlineAt = &value
	}
	if transaction.CompletedAt != nil {
		value := *transaction.CompletedAt
		copy.CompletedAt = &value
	}
	return &copy
}
