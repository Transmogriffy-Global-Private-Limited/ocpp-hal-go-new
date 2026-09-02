package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// V1TraceDelivery is the immutable wire record claimed by the diagnostic
// worker. It is independent of V1Fact and never participates in OCPP truth.
type V1TraceDelivery struct {
	SchemaVersion int            `json:"schema_version"`
	Trace         V1Trace        `json:"-"`
	Event         V1TraceEvent   `json:"-"`
	ContentSHA256 string         `json:"immutable_content_sha256"`
	Payload       map[string]any `json:"-"`
	ClaimToken    string         `json:"-"`
	Retries       int            `json:"-"`
}

func (delivery V1TraceDelivery) Envelope() map[string]any {
	if delivery.Payload != nil {
		return delivery.Payload
	}
	data, _ := delivery.Event.Data.(map[string]any)
	if data == nil {
		data = map[string]any{}
	}
	return map[string]any{
		"schema_version":           delivery.SchemaVersion,
		"trace_id":                 delivery.Trace.TraceID,
		"event_id":                 delivery.Event.EventID,
		"cpo_id":                   delivery.Trace.CPOID,
		"cms_start_intent_id":      nullableString(delivery.Trace.CMSStartIntentID),
		"cms_charging_session_id":  nullableString(delivery.Trace.CMSChargingSessionID),
		"cms_command_id":           nullableString(delivery.Trace.CMSCommandID),
		"hal_transaction_id":       nullableString(delivery.Trace.HALTransactionID),
		"ocpp_transaction_id":      delivery.Trace.OCPPTransactionID,
		"charger_ocpp_identity":    delivery.Trace.ChargerOCPPIdentity,
		"ocpp_connector_number":    delivery.Trace.OCPPConnectorNumber,
		"source":                   delivery.Event.Source,
		"target":                   delivery.Event.Target,
		"category":                 delivery.Event.Category,
		"protocol":                 delivery.Event.Protocol,
		"phase":                    delivery.Event.Phase,
		"summary":                  delivery.Event.Summary,
		"occurred_at":              delivery.Event.OccurredAt.UTC(),
		"state_before":             delivery.Event.StateBefore,
		"state_after":              delivery.Event.StateAfter,
		"correlation_id":           delivery.Event.CorrelationID,
		"data":                     data,
		"immutable_content_sha256": delivery.ContentSHA256,
	}
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func traceContentSHA256(trace V1Trace, event V1TraceEvent) (string, error) {
	delivery := V1TraceDelivery{SchemaVersion: 1, Trace: trace, Event: event}
	envelope := delivery.Envelope()
	delete(envelope, "immutable_content_sha256")
	canonical, err := canonicalV1JSON(envelope)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// V1TraceDeliveryStore intentionally has no V1Fact methods. A worker using
// this interface cannot consume authoritative fact capacity by construction.
type V1TraceDeliveryStore interface {
	ClaimV1TraceDeliveries(context.Context, time.Time, int) ([]V1TraceDelivery, error)
	MarkV1TraceDelivery(context.Context, string, string, int, bool, bool, string, time.Time) error
}

func (s *PostgresStore) ClaimV1TraceDeliveries(ctx context.Context, now time.Time, limit int) ([]V1TraceDelivery, error) {
	if limit < 1 {
		return nil, nil
	}
	claimToken, err := NewSecureUUIDString()
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `WITH due AS (
		SELECT event_id FROM v1_trace_delivery_outbox
		WHERE ((status IN ('PENDING','RETRY') AND next_retry_at <= $1)
			OR (status='DELIVERING' AND claimed_until < $1))
			AND (claimed_until IS NULL OR claimed_until < $1)
		ORDER BY next_retry_at, created_at FOR UPDATE SKIP LOCKED LIMIT $2
	)
	UPDATE v1_trace_delivery_outbox o SET status='DELIVERING', claimed_until=$1+INTERVAL '30 seconds', claim_token=$3::uuid
	FROM due
	WHERE o.event_id=due.event_id
	RETURNING o.event_id::text,o.payload::text,o.content_sha256,o.claim_token::text,o.retries`, now, limit, claimToken)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []V1TraceDelivery{}
	for rows.Next() {
		var delivery V1TraceDelivery
		var rawPayload []byte
		delivery.SchemaVersion = 1
		if err := rows.Scan(&delivery.Event.EventID, &rawPayload, &delivery.ContentSHA256, &delivery.ClaimToken, &delivery.Retries); err != nil {
			return nil, err
		}
		if len(rawPayload) == 0 {
			return nil, errors.New("persisted trace delivery payload is empty")
		}
		if err := json.Unmarshal(rawPayload, &delivery.Payload); err != nil {
			return nil, fmt.Errorf("decode persisted trace delivery payload: %w", err)
		}
		if payloadEventID, _ := delivery.Payload["event_id"].(string); payloadEventID != delivery.Event.EventID {
			return nil, errors.New("persisted trace delivery payload event identity mismatch")
		}
		out = append(out, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *PostgresStore) MarkV1TraceDelivery(ctx context.Context, eventID, claimToken string, statusCode int, success, terminal bool, detail string, next time.Time) error {
	status := "RETRY"
	if success {
		status = "DELIVERED"
	}
	if terminal {
		status = "RECONCILIATION_REQUIRED"
	}
	result, err := s.db.ExecContext(ctx, `UPDATE v1_trace_delivery_outbox SET status=$2,retries=CASE WHEN $2='DELIVERED' THEN retries ELSE retries+1 END,next_retry_at=$3,claimed_until=NULL,claim_token=NULL,delivery_status_code=$4,last_error=$5,sent_at=CASE WHEN $2='DELIVERED' THEN NOW() ELSE sent_at END WHERE event_id=$1::uuid AND status='DELIVERING' AND claim_token=$6::uuid`, eventID, status, next, statusCode, nullString(detail), claimToken)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("trace delivery claim lost")
	}
	return nil
}

var _ V1TraceDeliveryStore = (*PostgresStore)(nil)
