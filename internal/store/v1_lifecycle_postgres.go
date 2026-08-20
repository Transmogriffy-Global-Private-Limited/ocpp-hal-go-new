package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/gowebpki/jcs"
)

const v1FactProducer = "ocpp-hal-go-new"

// V1FactEnvelope is the exact immutable fact representation sent to the CMS.
// The digest field is omitted only while canonicalizing immutable content.
type V1FactEnvelope struct {
	FactID                 string          `json:"fact_id"`
	FactType               string          `json:"fact_type"`
	SchemaVersion          int             `json:"schema_version"`
	OccurredAt             time.Time       `json:"occurred_at"`
	Producer               string          `json:"producer"`
	ImmutableContentSHA256 string          `json:"immutable_content_sha256"`
	Payload                json.RawMessage `json:"payload"`
}

type v1ImmutableFactEnvelope struct {
	FactID        string          `json:"fact_id"`
	FactType      string          `json:"fact_type"`
	SchemaVersion int             `json:"schema_version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Producer      string          `json:"producer"`
	Payload       json.RawMessage `json:"payload"`
}

// Envelope returns the wire representation used for CMS fact delivery.
func (f V1Fact) Envelope() V1FactEnvelope {
	return V1FactEnvelope{
		FactID:                 f.FactID,
		FactType:               f.FactType,
		SchemaVersion:          f.SchemaVersion,
		OccurredAt:             f.OccurredAt,
		Producer:               f.Producer,
		ImmutableContentSHA256: f.ContentSHA256,
		Payload:                json.RawMessage(f.Payload),
	}
}

func (f V1Fact) immutableEnvelope() v1ImmutableFactEnvelope {
	return v1ImmutableFactEnvelope{
		FactID:        f.FactID,
		FactType:      f.FactType,
		SchemaVersion: f.SchemaVersion,
		OccurredAt:    f.OccurredAt,
		Producer:      f.Producer,
		Payload:       json.RawMessage(f.Payload),
	}
}

// normalizeV1FactOccurredAt matches PostgreSQL TIMESTAMPTZ's durable precision
// and produces the single representation used for hashing and delivery.
func normalizeV1FactOccurredAt(occurredAt time.Time) time.Time {
	return occurredAt.UTC().Truncate(time.Microsecond)
}

// canonicalV1JSON first establishes ordinary JSON representation, then applies
// RFC 8785 JSON Canonicalization Scheme to that JSON-compatible value.
func canonicalV1JSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(raw)
}

func (s *PostgresStore) insertV1FactTx(ctx context.Context, tx *sql.Tx, factType, aggregate string, sequence *int64, occurredAt time.Time, payload map[string]any) error {
	occurredAt = normalizeV1FactOccurredAt(occurredAt)
	body, err := canonicalV1JSON(payload)
	if err != nil {
		return err
	}
	fact := V1Fact{
		FactID:        NewUUIDString(),
		FactType:      factType,
		SchemaVersion: 1,
		OccurredAt:    occurredAt,
		Producer:      v1FactProducer,
		Payload:       body,
	}
	envelope, err := canonicalV1JSON(fact.immutableEnvelope())
	if err != nil {
		return err
	}
	digest := sha256.Sum256(envelope)
	_, err = tx.ExecContext(ctx, `INSERT INTO v1_fact_outbox (fact_id,fact_type,aggregate_key,sequence,payload,content_digest,schema_version,occurred_at,producer,status,next_retry_at) VALUES ($1,$2,$3,$4,$5::jsonb,$6,1,$7,$8,'PENDING',$7) ON CONFLICT (fact_type,aggregate_key,sequence) DO NOTHING`, fact.FactID, factType, aggregate, sequence, string(body), hex.EncodeToString(digest[:]), occurredAt, v1FactProducer)
	return err
}

func v1CommandFact(command *V1RemoteCommand) map[string]any {
	payload := map[string]any{
		"hal_command_id": command.HALCommandID, "cms_command_id": command.CMSCommandID,
		"kind": command.Kind, "state": command.State, "charger_ocpp_identity": command.ChargerOCPPIdentity,
		"ocpp_connector_number": command.OCPPConnectorNumber, "delivery_attempts": command.DeliveryAttempts,
		"ocpp_result":         nullString(command.LastOCPPResult),
		"last_error_category": nullString(command.LastErrorCategory),
		"last_error_detail":   nullString(command.LastErrorDetail),
		"occurred_at":         command.UpdatedAt,
	}
	if command.HALTransactionID != "" {
		payload["hal_transaction_id"] = command.HALTransactionID
	}
	if command.OCPPTransactionID != nil {
		payload["ocpp_transaction_id"] = *command.OCPPTransactionID
	}
	return payload
}

func v1StartedFact(t *V1Transaction, commandID string) map[string]any {
	return map[string]any{
		"hal_transaction_id": t.HALTransactionID, "ocpp_transaction_id": t.OCPPTransactionID,
		"hal_command_id": commandID, "cms_command_id": t.CMSCommandID,
		"cms_start_intent_id": t.CMSStartIntentID, "cpo_id": t.CPOID,
		"cms_charger_id": t.CMSChargerID, "cms_connector_id": t.CMSConnectorID,
		"charger_ocpp_identity": t.ChargerOCPPIdentity, "ocpp_connector_number": t.OCPPConnectorNumber,
		"id_tag": t.IDTag, "meter_start_wh": t.MeterStartWh, "started_at": t.ActualStartedAt,
	}
}

func v1MeterFact(t *V1Transaction) map[string]any {
	return map[string]any{
		"hal_transaction_id": t.HALTransactionID, "ocpp_transaction_id": t.OCPPTransactionID,
		"cms_start_intent_id": t.CMSStartIntentID, "cms_charger_id": t.CMSChargerID,
		"cms_connector_id": t.CMSConnectorID, "charger_ocpp_identity": t.ChargerOCPPIdentity,
		"ocpp_connector_number": t.OCPPConnectorNumber, "meter_sequence": t.MeterSequence,
		"meter_value_wh": *t.LatestMeterWh, "consumed_wh": *t.LatestMeterWh - t.MeterStartWh,
		"meter_observed_at": *t.MeterObservedAt,
	}
}

func v1CompletedFact(t *V1Transaction, command *V1RemoteCommand) map[string]any {
	payload := map[string]any{
		"hal_transaction_id": t.HALTransactionID, "ocpp_transaction_id": t.OCPPTransactionID,
		"hal_command_id": nil, "cms_command_id": nil,
		"cms_start_intent_id": t.CMSStartIntentID, "cms_charger_id": t.CMSChargerID,
		"cms_connector_id": t.CMSConnectorID, "charger_ocpp_identity": t.ChargerOCPPIdentity,
		"ocpp_connector_number": t.OCPPConnectorNumber, "meter_start_wh": t.MeterStartWh,
		"meter_stop_wh": *t.MeterStopWh, "stopped_at": *t.CompletedAt,
		"ocpp_stop_reason":         nullString(t.OCPPStopReason),
		"requested_stop_initiator": nil, "requested_stop_reason": nil,
	}
	if command != nil {
		payload["hal_command_id"] = command.HALCommandID
		payload["cms_command_id"] = command.CMSCommandID
	}
	if t.RequestedStopInitiator != "" {
		payload["requested_stop_initiator"] = t.RequestedStopInitiator
	}
	if t.RequestedStopReason != "" {
		payload["requested_stop_reason"] = t.RequestedStopReason
	}
	return payload
}

func v1ConnectionFact(cpoID, cmsChargerID, identity, state string, generation, sequence int64, observedAt time.Time) map[string]any {
	return map[string]any{"cpo_id": cpoID, "cms_charger_id": cmsChargerID, "charger_ocpp_identity": identity, "connection_state": state, "connection_generation": generation, "connection_sequence": sequence, "observed_at": observedAt}
}

func v1StatusFact(runtime *V1ConnectorRuntime) map[string]any {
	payload := map[string]any{"cpo_id": runtime.CPOID, "cms_charger_id": runtime.CMSChargerID, "cms_connector_id": runtime.CMSConnectorID, "charger_ocpp_identity": runtime.ChargerOCPPIdentity, "ocpp_connector_number": runtime.OCPPConnectorNumber, "ocpp_connector_status": runtime.Status, "connector_status_sequence": runtime.StatusSequence, "observed_at": *runtime.ObservedAt}
	if runtime.ErrorCode != "" {
		payload["error_code"] = runtime.ErrorCode
	}
	if runtime.Info != "" {
		payload["info"] = runtime.Info
	}
	if runtime.VendorID != "" {
		payload["vendor_id"] = runtime.VendorID
	}
	if runtime.VendorErrorCode != "" {
		payload["vendor_error_code"] = runtime.VendorErrorCode
	}
	return payload
}

func (s *PostgresStore) GetV1TransactionByOCPP(ctx context.Context, identity string, ocppID int64) (*V1Transaction, error) {
	var halID string
	err := s.db.QueryRowContext(ctx, `SELECT hal_transaction_id::text FROM v1_transactions WHERE charger_ocpp_identity=$1 AND ocpp_transaction_id=$2`, identity, ocppID).Scan(&halID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrV1TransactionNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.GetV1Transaction(ctx, halID)
}

func (s *PostgresStore) UpdateV1MeterForOCPP(ctx context.Context, identity string, ocppID, meterWh int64, observedAt time.Time) (*V1Transaction, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var halID string
	var meterStart int64
	var latest sql.NullInt64
	var completed, previousObserved sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT hal_transaction_id::text,meter_start_wh,latest_meter_wh,completed_at,meter_observed_at FROM v1_transactions WHERE charger_ocpp_identity=$1 AND ocpp_transaction_id=$2 FOR UPDATE`, identity, ocppID).Scan(&halID, &meterStart, &latest, &completed, &previousObserved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrV1TransactionNotFound
	}
	if err != nil {
		return nil, false, err
	}
	if completed.Valid || meterWh < meterStart || (latest.Valid && meterWh < latest.Int64) {
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		current, getErr := s.GetV1Transaction(ctx, halID)
		return current, false, getErr
	}
	if previousObserved.Valid && observedAt.Before(previousObserved.Time) {
		observedAt = previousObserved.Time
	}
	_, err = tx.ExecContext(ctx, `UPDATE v1_transactions SET latest_meter_wh=$2,meter_observed_at=$3,meter_sequence=meter_sequence+1,updated_at=NOW() WHERE hal_transaction_id=$1`, halID, meterWh, observedAt)
	if err != nil {
		return nil, false, err
	}
	current, err := s.getV1TransactionByID(ctx, txByQueryer{tx}, halID)
	if err != nil {
		return nil, false, err
	}
	seq := current.MeterSequence
	if err := s.insertV1FactTx(ctx, tx, "transaction.meter", halID, &seq, observedAt, v1MeterFact(current)); err != nil {
		return nil, false, err
	}
	if current.EnergyLimitWh != nil && meterWh-current.MeterStartWh >= *current.EnergyLimitWh {
		if _, _, err := s.ensureV1StopWorkflowTx(ctx, tx, current, "ENERGY_LIMIT", "energy_limit_reached"); err != nil {
			return nil, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return current, true, nil
}

func (s *PostgresStore) UpdateV1Meter(ctx context.Context, halID string, ocppID, meterWh int64, observedAt time.Time) (*V1Transaction, error) {
	t, err := s.GetV1Transaction(ctx, halID)
	if err != nil {
		return nil, err
	}
	if t.OCPPTransactionID != ocppID {
		return nil, ErrV1TransactionNotFound
	}
	updated, _, err := s.UpdateV1MeterForOCPP(ctx, t.ChargerOCPPIdentity, ocppID, meterWh, observedAt)
	return updated, err
}

func (s *PostgresStore) ensureV1StopWorkflowTx(ctx context.Context, tx *sql.Tx, t *V1Transaction, initiator, reason string) (*V1StopWorkflow, bool, error) {
	workflow := &V1StopWorkflow{}
	err := tx.QueryRowContext(ctx, `SELECT hal_transaction_id::text,requested_stop_initiator,requested_stop_reason,state,delivery_attempts,COALESCE(last_ocpp_result,''),COALESCE(last_error_category,''),COALESCE(last_error_detail,''),created_at,updated_at,completed_at FROM v1_stop_workflows WHERE hal_transaction_id=$1 FOR UPDATE`, t.HALTransactionID).Scan(&workflow.HALTransactionID, &workflow.RequestedStopInitiator, &workflow.RequestedStopReason, &workflow.State, &workflow.DeliveryAttempts, &workflow.LastOCPPResult, &workflow.LastErrorCategory, &workflow.LastErrorDetail, &workflow.CreatedAt, &workflow.UpdatedAt, &workflow.CompletedAt)
	if err == nil {
		return workflow, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	if t.CompletedAt != nil {
		return nil, false, ErrV1TransactionNotFound
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO v1_stop_workflows (hal_transaction_id,requested_stop_initiator,requested_stop_reason,state) VALUES ($1,$2,$3,'PERSISTED')`, t.HALTransactionID, initiator, reason)
	if err != nil {
		return nil, false, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE v1_transactions SET stop_state='PERSISTED',requested_stop_initiator=$2,requested_stop_reason=$3,updated_at=NOW() WHERE hal_transaction_id=$1`, t.HALTransactionID, initiator, reason)
	if err != nil {
		return nil, false, err
	}
	return &V1StopWorkflow{HALTransactionID: t.HALTransactionID, RequestedStopInitiator: initiator, RequestedStopReason: reason, State: "PERSISTED"}, true, nil
}

func (s *PostgresStore) EnsureV1StopWorkflow(ctx context.Context, halID, initiator, reason string) (*V1StopWorkflow, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	t, err := s.getV1TransactionByID(ctx, txByQueryer{tx}, halID)
	if err != nil {
		return nil, false, err
	}
	w, created, err := s.ensureV1StopWorkflowTx(ctx, tx, t, initiator, reason)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return w, created, nil
}

func (s *PostgresStore) RequestV1Stop(ctx context.Context, halID, initiator, reason string) (*V1Transaction, bool, error) {
	_, created, err := s.EnsureV1StopWorkflow(ctx, halID, initiator, reason)
	if err != nil {
		return nil, false, err
	}
	t, err := s.GetV1Transaction(ctx, halID)
	return t, created, err
}

func (s *PostgresStore) GetV1StopWorkflow(ctx context.Context, halID string) (*V1StopWorkflow, error) {
	w := &V1StopWorkflow{}
	var completed sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT hal_transaction_id::text,requested_stop_initiator,requested_stop_reason,state,delivery_attempts,COALESCE(last_ocpp_result,''),COALESCE(last_error_category,''),COALESCE(last_error_detail,''),created_at,updated_at,completed_at FROM v1_stop_workflows WHERE hal_transaction_id=$1`, halID).Scan(&w.HALTransactionID, &w.RequestedStopInitiator, &w.RequestedStopReason, &w.State, &w.DeliveryAttempts, &w.LastOCPPResult, &w.LastErrorCategory, &w.LastErrorDetail, &w.CreatedAt, &w.UpdatedAt, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrV1TransactionNotFound
	}
	if err != nil {
		return nil, err
	}
	if completed.Valid {
		w.CompletedAt = &completed.Time
	}
	return w, nil
}

func (s *PostgresStore) ClaimV1StopDelivery(ctx context.Context, halID string) (*V1StopWorkflow, bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE v1_stop_workflows w SET state='PENDING_DELIVERY',claimed_until=NOW()+INTERVAL '30 seconds',updated_at=NOW() WHERE w.hal_transaction_id=$1 AND w.state='PERSISTED' AND EXISTS (SELECT 1 FROM v1_transactions t WHERE t.hal_transaction_id=w.hal_transaction_id AND t.completed_at IS NULL)`, halID)
	if err != nil {
		return nil, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	w, err := s.GetV1StopWorkflow(ctx, halID)
	return w, rows == 1, err
}

func (s *PostgresStore) BeginV1StopDelivery(ctx context.Context, halID string) (*V1StopWorkflow, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE v1_stop_workflows SET state='DELIVERY_ATTEMPTED',delivery_attempts=delivery_attempts+1,claimed_until=NOW()+INTERVAL '2 minutes',updated_at=NOW() WHERE hal_transaction_id=$1 AND state='PENDING_DELIVERY'`, halID)
	if err != nil {
		return nil, err
	}
	return s.GetV1StopWorkflow(ctx, halID)
}

func (s *PostgresStore) MarkV1StopDelivery(ctx context.Context, halID, status, result, detail string) (*V1StopWorkflow, error) {
	state, category := "DELIVERY_ATTEMPTED", ""
	switch status {
	case "Accepted":
		state = "OCPP_ACCEPTED"
	case "Rejected":
		state = "OCPP_REJECTED"
		category = "ocpp"
	case "AMBIGUOUS":
		state = "AMBIGUOUS"
		category = "delivery"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE v1_stop_workflows SET state=$2,last_ocpp_result=$3,last_error_category=$4,last_error_detail=$5,claimed_until=NULL,updated_at=NOW() WHERE hal_transaction_id=$1 AND state NOT IN ('COMPLETED','SUPERSEDED')`, halID, state, nullString(result), nullString(category), nullString(detail))
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE v1_transactions SET stop_state=$2,updated_at=NOW() WHERE hal_transaction_id=$1 AND completed_at IS NULL`, halID, state)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `UPDATE v1_remote_commands SET state=$2,last_ocpp_result=$3,last_error_category=$4,last_error_detail=$5,claimed_until=NULL,updated_at=NOW() WHERE stop_workflow_transaction_id=$1 AND kind='STOP' AND state NOT IN ('SUPERSEDED','MATERIALIZED') RETURNING cms_command_id::text`, halID, state, nullString(result), nullString(category), nullString(detail))
	if err != nil {
		return nil, err
	}
	var commandIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		commandIDs = append(commandIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, commandID := range commandIDs {
		command, err := s.getV1CommandWith(ctx, txByQueryer{tx}, commandID)
		if err != nil {
			return nil, err
		}
		if err := s.insertV1FactTx(ctx, tx, "command.updated", command.HALCommandID, nil, command.UpdatedAt, v1CommandFact(command)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetV1StopWorkflow(ctx, halID)
}

func (s *PostgresStore) RecoverV1CommandDelivery(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE v1_remote_commands SET state='PERSISTED',claimed_until=NULL,updated_at=NOW() WHERE kind='START' AND state='PENDING_DELIVERY'`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE v1_remote_commands SET state='AMBIGUOUS',last_error_category='recovery',last_error_detail='process interruption after durable delivery attempt',claimed_until=NULL,updated_at=NOW() WHERE kind='START' AND state='DELIVERY_ATTEMPTED'`)
	return err
}

func (s *PostgresStore) RecoverV1StopDelivery(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE v1_stop_workflows SET state='PERSISTED',claimed_until=NULL,updated_at=NOW() WHERE state='PENDING_DELIVERY'`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE v1_stop_workflows SET state='AMBIGUOUS',last_error_category='recovery',last_error_detail='process interruption after durable delivery attempt',claimed_until=NULL,updated_at=NOW() WHERE state='DELIVERY_ATTEMPTED'`)
	return err
}

// ListV1DispatchableStops returns only durable workflows whose delivery is
// proven not to have crossed the OCPP network boundary.
func (s *PostgresStore) ListV1DispatchableStops(ctx context.Context, limit int) ([]*V1StopWorkflow, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT w.hal_transaction_id::text,w.requested_stop_initiator,w.requested_stop_reason,w.state,w.delivery_attempts,COALESCE(w.last_ocpp_result,''),COALESCE(w.last_error_category,''),COALESCE(w.last_error_detail,''),w.created_at,w.updated_at,w.completed_at FROM v1_stop_workflows w JOIN v1_transactions t ON t.hal_transaction_id=w.hal_transaction_id WHERE w.state='PERSISTED' AND t.completed_at IS NULL ORDER BY w.created_at,w.hal_transaction_id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	workflows := make([]*V1StopWorkflow, 0)
	for rows.Next() {
		workflow := &V1StopWorkflow{}
		var completed sql.NullTime
		if err := rows.Scan(&workflow.HALTransactionID, &workflow.RequestedStopInitiator, &workflow.RequestedStopReason, &workflow.State, &workflow.DeliveryAttempts, &workflow.LastOCPPResult, &workflow.LastErrorCategory, &workflow.LastErrorDetail, &workflow.CreatedAt, &workflow.UpdatedAt, &completed); err != nil {
			return nil, err
		}
		if completed.Valid {
			workflow.CompletedAt = &completed.Time
		}
		workflows = append(workflows, workflow)
	}
	return workflows, rows.Err()
}

func (s *PostgresStore) ListV1OverdueTransactions(ctx context.Context, now time.Time, limit int) ([]*V1Transaction, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT hal_transaction_id::text FROM v1_transactions WHERE completed_at IS NULL AND stop_deadline_at IS NOT NULL AND stop_deadline_at <= $1 ORDER BY stop_deadline_at LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*V1Transaction
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		t, err := s.GetV1Transaction(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *PostgresStore) CompleteV1Transaction(ctx context.Context, halID string, meterStopWh int64, ocppReason string, completedAt, observedAt time.Time) (*V1Transaction, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t, err := s.getV1TransactionByID(ctx, txByQueryer{tx}, halID)
	if err != nil {
		return nil, err
	}
	if t.CompletedAt != nil {
		if t.MeterStopWh == nil || *t.MeterStopWh != meterStopWh || t.OCPPStopReason != ocppReason || !t.CompletedAt.Equal(completedAt) {
			return nil, ErrV1InvalidEvidence
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return t, nil
	}
	if meterStopWh < t.MeterStartWh || (t.LatestMeterWh != nil && meterStopWh < *t.LatestMeterWh) || completedAt.Before(t.ActualStartedAt) || observedAt.Before(t.ObservedStartedAt) || !plausibleV1ProtocolTime(completedAt, observedAt) {
		return nil, ErrV1InvalidEvidence
	}
	meterObservedAt := completedAt
	if t.MeterObservedAt != nil && meterObservedAt.Before(*t.MeterObservedAt) {
		meterObservedAt = *t.MeterObservedAt
	}
	_, err = tx.ExecContext(ctx, `UPDATE v1_transactions SET meter_stop_wh=$2,latest_meter_wh=$2,meter_observed_at=$3,ocpp_stop_reason=$4,completed_at=$5,observed_completed_at=$6,stop_state='COMPLETED',updated_at=NOW() WHERE hal_transaction_id=$1`, halID, meterStopWh, meterObservedAt, nullString(ocppReason), completedAt, observedAt)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE v1_stop_workflows SET state='COMPLETED',completed_at=$2,claimed_until=NULL,updated_at=NOW() WHERE hal_transaction_id=$1 AND state <> 'COMPLETED'`, halID, completedAt)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE v1_remote_commands SET state='SUPERSEDED',updated_at=NOW() WHERE stop_workflow_transaction_id=$1 AND state NOT IN ('SUPERSEDED','MATERIALIZED')`, halID)
	if err != nil {
		return nil, err
	}
	completed, err := s.getV1TransactionByID(ctx, txByQueryer{tx}, halID)
	if err != nil {
		return nil, err
	}
	command, err := s.getV1CompletionCommandTx(ctx, tx, halID)
	if err != nil {
		return nil, err
	}
	if err := s.insertV1FactTx(ctx, tx, "transaction.completed", halID, nil, observedAt, v1CompletedFact(completed, command)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return completed, nil
}

func (s *PostgresStore) getV1CompletionCommandTx(ctx context.Context, tx *sql.Tx, halTransactionID string) (*V1RemoteCommand, error) {
	var cmsCommandID string
	err := tx.QueryRowContext(ctx, `SELECT cms_command_id::text FROM v1_remote_commands WHERE kind='STOP' AND stop_workflow_transaction_id=$1 ORDER BY created_at,id LIMIT 1`, halTransactionID).Scan(&cmsCommandID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.getV1CommandWith(ctx, txByQueryer{tx}, cmsCommandID)
}

func (s *PostgresStore) ClaimV1Facts(ctx context.Context, now time.Time, limit int) ([]V1Fact, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `WITH due AS (SELECT fact_id FROM v1_fact_outbox WHERE ((status IN ('PENDING','RETRY') AND next_retry_at <= $1) OR (status='DELIVERING' AND claimed_until < $1)) AND (claimed_until IS NULL OR claimed_until < $1) ORDER BY next_retry_at,created_at FOR UPDATE SKIP LOCKED LIMIT $2) UPDATE v1_fact_outbox f SET status='DELIVERING',claimed_until=$1+INTERVAL '30 seconds' FROM due WHERE f.fact_id=due.fact_id RETURNING f.fact_id::text,f.fact_type,f.schema_version,f.occurred_at,f.producer,f.content_digest,f.payload::text,f.status,f.retries,f.next_retry_at,f.claimed_until,f.delivery_status_code,COALESCE(f.last_error,'')`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []V1Fact
	for rows.Next() {
		var f V1Fact
		var claim sql.NullTime
		var status sql.NullInt64
		if err := rows.Scan(&f.FactID, &f.FactType, &f.SchemaVersion, &f.OccurredAt, &f.Producer, &f.ContentSHA256, &f.Payload, &f.Status, &f.Retries, &f.NextRetryAt, &claim, &status, &f.LastError); err != nil {
			return nil, err
		}
		f.OccurredAt = normalizeV1FactOccurredAt(f.OccurredAt)
		if claim.Valid {
			f.ClaimedUntil = &claim.Time
		}
		if status.Valid {
			v := int(status.Int64)
			f.DeliveryStatus = &v
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *PostgresStore) MarkV1FactDelivery(ctx context.Context, factID string, statusCode int, success, terminal bool, detail string, next time.Time) error {
	state := "RETRY"
	if success {
		state = "DELIVERED"
	}
	if terminal {
		state = "RECONCILIATION_REQUIRED"
	}
	_, err := s.db.ExecContext(ctx, `UPDATE v1_fact_outbox SET status=$2,retries=CASE WHEN $2='DELIVERED' THEN retries ELSE retries+1 END,next_retry_at=$3,claimed_until=NULL,delivery_status_code=$4,last_error=$5,sent_at=CASE WHEN $2='DELIVERED' THEN NOW() ELSE sent_at END WHERE fact_id=$1`, factID, state, next, statusCode, nullString(detail))
	return err
}
