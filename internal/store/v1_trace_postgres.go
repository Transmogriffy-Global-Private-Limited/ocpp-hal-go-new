package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (s *PostgresStore) EnsureV1Trace(ctx context.Context, trace V1Trace) (*V1Trace, error) {
	if trace.CreatedAt.IsZero() {
		trace.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO v1_charging_traces (trace_id,cpo_id,cms_start_intent_id,cms_charging_session_id,cms_command_id,hal_transaction_id,ocpp_transaction_id,charger_ocpp_identity,ocpp_connector_number,created_at) VALUES ($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,NULLIF($6,'')::uuid,$7,$8,$9,$10) ON CONFLICT (trace_id) DO NOTHING`, trace.TraceID, trace.CPOID, trace.CMSStartIntentID, trace.CMSChargingSessionID, trace.CMSCommandID, trace.HALTransactionID, trace.OCPPTransactionID, trace.ChargerOCPPIdentity, trace.OCPPConnectorNumber, trace.CreatedAt)
	if err != nil {
		return nil, err
	}
	return s.GetV1Trace(ctx, trace.TraceID)
}

func (s *PostgresStore) BindV1TraceTransaction(ctx context.Context, traceID string, transaction *V1Transaction) error {
	result, err := s.db.ExecContext(ctx, `UPDATE v1_charging_traces SET hal_transaction_id=$2::uuid,ocpp_transaction_id=$3 WHERE trace_id=$1::uuid AND (hal_transaction_id IS NULL OR hal_transaction_id=$2::uuid)`, traceID, transaction.HALTransactionID, transaction.OCPPTransactionID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrV1InvalidEvidence
	}
	return nil
}

func (s *PostgresStore) EnsureV1TraceForTransaction(ctx context.Context, transaction *V1Transaction) (*V1Trace, error) {
	trace, err := s.FindV1TraceByTransaction(ctx, transaction.HALTransactionID)
	if err == nil {
		return trace, nil
	}
	if !errors.Is(err, ErrV1TransactionNotFound) {
		return nil, err
	}
	if transaction.CMSStartIntentID != "" {
		trace, err = s.scanV1Trace(s.db.QueryRowContext(ctx, `SELECT trace_id::text,cpo_id::text,COALESCE(cms_start_intent_id::text,''),COALESCE(cms_charging_session_id::text,''),COALESCE(cms_command_id::text,''),COALESCE(hal_transaction_id::text,''),ocpp_transaction_id,charger_ocpp_identity,ocpp_connector_number,created_at FROM v1_charging_traces WHERE cms_start_intent_id=$1::uuid`, transaction.CMSStartIntentID))
		if err == nil {
			if err := s.BindV1TraceTransaction(ctx, trace.TraceID, transaction); err != nil {
				return nil, err
			}
			return s.GetV1Trace(ctx, trace.TraceID)
		}
		if !errors.Is(err, ErrV1TransactionNotFound) {
			return nil, err
		}
	}
	id, err := NewSecureUUIDString()
	if err != nil {
		return nil, err
	}
	ocpp := transaction.OCPPTransactionID
	return s.EnsureV1Trace(ctx, V1Trace{TraceID: id, CPOID: transaction.CPOID, CMSStartIntentID: transaction.CMSStartIntentID, CMSCommandID: transaction.CMSCommandID, HALTransactionID: transaction.HALTransactionID, OCPPTransactionID: &ocpp, ChargerOCPPIdentity: transaction.ChargerOCPPIdentity, OCPPConnectorNumber: transaction.OCPPConnectorNumber})
}

func (s *PostgresStore) FindV1TraceByTransaction(ctx context.Context, transactionID string) (*V1Trace, error) {
	return s.scanV1Trace(s.db.QueryRowContext(ctx, `SELECT trace_id::text,cpo_id::text,COALESCE(cms_start_intent_id::text,''),COALESCE(cms_charging_session_id::text,''),COALESCE(cms_command_id::text,''),COALESCE(hal_transaction_id::text,''),ocpp_transaction_id,charger_ocpp_identity,ocpp_connector_number,created_at FROM v1_charging_traces WHERE hal_transaction_id=$1::uuid`, transactionID))
}

// Correlation is connector-aware. It prefers an active transaction, then a
// most-recent pending CMS start. This deliberately never uses charger-only
// mutable state.
func (s *PostgresStore) FindV1TraceForConnector(ctx context.Context, identity string, connector int) (*V1Trace, error) {
	return s.scanV1Trace(s.db.QueryRowContext(ctx, `SELECT t.trace_id::text,t.cpo_id::text,COALESCE(t.cms_start_intent_id::text,''),COALESCE(t.cms_charging_session_id::text,''),COALESCE(t.cms_command_id::text,''),COALESCE(t.hal_transaction_id::text,''),t.ocpp_transaction_id,t.charger_ocpp_identity,t.ocpp_connector_number,t.created_at FROM v1_charging_traces t LEFT JOIN v1_transactions x ON x.hal_transaction_id=t.hal_transaction_id WHERE t.charger_ocpp_identity=$1 AND t.ocpp_connector_number=$2 AND (t.hal_transaction_id IS NULL OR x.completed_at IS NULL OR x.completed_at >= NOW() - INTERVAL '15 minutes') ORDER BY CASE WHEN x.completed_at IS NULL AND x.hal_transaction_id IS NOT NULL THEN 0 WHEN t.hal_transaction_id IS NULL THEN 1 ELSE 2 END,t.created_at DESC LIMIT 1`, identity, connector))
}

func (s *PostgresStore) AppendV1TraceEvent(ctx context.Context, traceID string, input V1TraceEventInput) error {
	id, err := NewSecureUUIDString()
	if err != nil {
		return err
	}
	when := input.OccurredAt
	if when.IsZero() {
		when = time.Now().UTC()
	}
	data, err := json.Marshal(sanitizeV1TraceData(input.Data))
	if err != nil {
		return err
	}
	if len(data) == 0 || string(data) == "null" {
		data = json.RawMessage(`{}`)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO v1_charging_trace_events (event_id,trace_id,source,target,category,protocol,phase,summary,occurred_at,state_before,state_after,correlation_id,data) VALUES ($1,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb)`, id, traceID, input.Source, input.Target, input.Category, input.Protocol, input.Phase, input.Summary, when, input.StateBefore, input.StateAfter, input.CorrelationID, data)
	return err
}

func (s *PostgresStore) GetV1Trace(ctx context.Context, traceID string) (*V1Trace, error) {
	return s.scanV1Trace(s.db.QueryRowContext(ctx, `SELECT trace_id::text,cpo_id::text,COALESCE(cms_start_intent_id::text,''),COALESCE(cms_charging_session_id::text,''),COALESCE(cms_command_id::text,''),COALESCE(hal_transaction_id::text,''),ocpp_transaction_id,charger_ocpp_identity,ocpp_connector_number,created_at FROM v1_charging_traces WHERE trace_id=$1::uuid`, traceID))
}
func (s *PostgresStore) scanV1Trace(row *sql.Row) (*V1Trace, error) {
	trace := &V1Trace{}
	var ocpp sql.NullInt64
	err := row.Scan(&trace.TraceID, &trace.CPOID, &trace.CMSStartIntentID, &trace.CMSChargingSessionID, &trace.CMSCommandID, &trace.HALTransactionID, &ocpp, &trace.ChargerOCPPIdentity, &trace.OCPPConnectorNumber, &trace.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrV1TransactionNotFound
	}
	if err != nil {
		return nil, err
	}
	if ocpp.Valid {
		value := ocpp.Int64
		trace.OCPPTransactionID = &value
	}
	return trace, nil
}
func (s *PostgresStore) ListV1TraceEvents(ctx context.Context, traceID string, before time.Time, beforeID string, limit int) ([]V1TraceEvent, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	args := []any{traceID}
	where := "trace_id=$1::uuid"
	if !before.IsZero() {
		where += " AND (occurred_at,event_id) < ($2,$3::uuid)"
		args = append(args, before, beforeID)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT event_id::text,trace_id::text,source,target,category,protocol,phase,summary,occurred_at,recorded_at,state_before,state_after,correlation_id,data FROM v1_charging_trace_events WHERE `+where+` ORDER BY occurred_at DESC,event_id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []V1TraceEvent{}
	for rows.Next() {
		var e V1TraceEvent
		if err := rows.Scan(&e.EventID, &e.TraceID, &e.Source, &e.Target, &e.Category, &e.Protocol, &e.Phase, &e.Summary, &e.OccurredAt, &e.RecordedAt, &e.StateBefore, &e.StateAfter, &e.CorrelationID, &e.Data); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteV1TracesBefore removes a bounded batch of diagnostic roots. The
// foreign-key cascade removes only their evidence rows; transaction, command,
// fact, connector, and billing state remain independent authorities.
func (s *PostgresStore) DeleteV1TracesBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit < 1 || limit > 1000 {
		return 0, ErrV1InvalidEvidence
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM v1_charging_traces WHERE trace_id IN (SELECT trace_id FROM v1_charging_traces WHERE created_at < $1 ORDER BY created_at ASC LIMIT $2)`, before, limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

var _ V1TraceStore = (*PostgresStore)(nil)
