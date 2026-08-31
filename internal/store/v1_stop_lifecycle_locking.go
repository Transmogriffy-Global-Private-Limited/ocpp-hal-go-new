package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	v1StopLifecycleTransactionAttempts = 3
	v1StopLifecycleRetryDelay          = 5 * time.Millisecond
)

type v1StopLifecycleRows struct {
	Transaction *V1Transaction
	Workflow    *V1StopWorkflow
}

// lockV1StopLifecycleRows makes the shared STOP lifecycle order explicit:
// transaction -> workflow -> remote commands -> facts. RemoteStop confirmation
// and charger StopTransaction evidence can arrive concurrently, so reversing
// the first two rows can deadlock PostgreSQL and reject valid completion truth.
func (s *PostgresStore) lockV1StopLifecycleRows(ctx context.Context, tx *sql.Tx, halTransactionID string) (v1StopLifecycleRows, error) {
	transaction, err := s.getV1TransactionBy(ctx, txByQueryer{tx}, "hal_transaction_id=$1 FOR UPDATE", halTransactionID)
	if err != nil {
		return v1StopLifecycleRows{}, err
	}

	workflow := &V1StopWorkflow{}
	var completed sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT hal_transaction_id::text,requested_stop_initiator,requested_stop_reason,state,delivery_attempts,COALESCE(last_ocpp_result,''),COALESCE(last_error_category,''),COALESCE(last_error_detail,''),created_at,updated_at,completed_at FROM v1_stop_workflows WHERE hal_transaction_id=$1 FOR UPDATE`, halTransactionID).Scan(&workflow.HALTransactionID, &workflow.RequestedStopInitiator, &workflow.RequestedStopReason, &workflow.State, &workflow.DeliveryAttempts, &workflow.LastOCPPResult, &workflow.LastErrorCategory, &workflow.LastErrorDetail, &workflow.CreatedAt, &workflow.UpdatedAt, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return v1StopLifecycleRows{Transaction: transaction}, nil
	}
	if err != nil {
		return v1StopLifecycleRows{}, err
	}
	if completed.Valid {
		workflow.CompletedAt = &completed.Time
	}
	return v1StopLifecycleRows{Transaction: transaction, Workflow: workflow}, nil
}

func retryV1StopLifecycleTransaction(ctx context.Context, operation func() error) error {
	var lastErr error
	for attempt := 0; attempt < v1StopLifecycleTransactionAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := operation()
		if err == nil {
			return nil
		}
		if !isRetryableV1StopLifecycleTransactionError(err) || attempt+1 == v1StopLifecycleTransactionAttempts {
			return err
		}
		lastErr = err
		timer := time.NewTimer(v1StopLifecycleRetryDelay * time.Duration(attempt+1))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

// withV1StopLifecycleTransaction retries only SQLSTATEs for which PostgreSQL
// has aborted the local transaction. It never retries an OCPP network command
// and never treats an unknown commit error as safe to replay.
func (s *PostgresStore) withV1StopLifecycleTransaction(ctx context.Context, operation func(*sql.Tx) error) error {
	return retryV1StopLifecycleTransaction(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := operation(tx); err != nil {
			return err
		}
		return tx.Commit()
	})
}

func isRetryableV1StopLifecycleTransactionError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "40001")
}
