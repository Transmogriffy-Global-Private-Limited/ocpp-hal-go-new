package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestV1StopLifecycleRetryIsBoundedToPostgresAbortStates(t *testing.T) {
	deadlock := &pgconn.PgError{Code: "40P01"}
	for name, test := range map[string]struct {
		errors   []error
		attempts int
		wantErr  bool
	}{
		"deadlock retries then succeeds":      {errors: []error{deadlock, deadlock, nil}, attempts: 3},
		"serialization retries then succeeds": {errors: []error{&pgconn.PgError{Code: "40001"}, nil}, attempts: 2},
		"non database error is not retried":   {errors: []error{errors.New("validation")}, attempts: 1, wantErr: true},
		"retry budget is bounded":             {errors: []error{deadlock, deadlock, deadlock}, attempts: v1StopLifecycleTransactionAttempts, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			attempt := 0
			err := retryV1StopLifecycleTransaction(context.Background(), func() error {
				result := test.errors[attempt]
				attempt++
				return result
			})
			if attempt != test.attempts {
				t.Fatalf("attempts=%d, want %d", attempt, test.attempts)
			}
			if (err != nil) != test.wantErr {
				t.Fatalf("err=%v, want error=%v", err, test.wantErr)
			}
		})
	}
}

func TestV1StopLifecycleRetryClassifiesOnlyDeadlockAndSerializationFailures(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "deadlock", err: &pgconn.PgError{Code: "40P01"}, want: true},
		{name: "serialization", err: &pgconn.PgError{Code: "40001"}, want: true},
		{name: "constraint", err: &pgconn.PgError{Code: "23505"}, want: false},
		{name: "ordinary", err: errors.New("ordinary failure"), want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isRetryableV1StopLifecycleTransactionError(test.err); got != test.want {
				t.Fatalf("retryable=%v, want %v", got, test.want)
			}
		})
	}
}

func TestV1CompletionFactUniquenessMigrationIsAdditiveAndTerminal(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "migrations", "017_enforce_v1_transaction_completion_fact_uniqueness.sql"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS v1_transaction_completion_fact_keys",
		"aggregate_key TEXT PRIMARY KEY",
		"fact_id UUID NULL UNIQUE REFERENCES v1_fact_outbox(fact_id)",
		"SELECT DISTINCT ON (aggregate_key) aggregate_key, fact_id",
		"WHERE fact_type = 'transaction.completed'",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration is missing %q:\n%s", required, text)
		}
	}
}
