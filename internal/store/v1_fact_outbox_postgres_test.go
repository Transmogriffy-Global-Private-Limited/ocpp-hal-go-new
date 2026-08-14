package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
)

func TestV1FactOutboxPostgresRoundTripPreservesImmutableDigest(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for the disposable PostgreSQL fact-outbox regression test")
	}
	ctx := context.Background()
	s, err := NewPostgresStore(config.Config{DatabaseURL: dsn})
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()

	occurredAt := time.Date(2026, 8, 13, 12, 1, 37, 155787095, time.UTC)
	factID := insertFactOutboxFixtureAt(t, s, "digest-round-trip", occurredAt)
	defer deleteFactOutboxFixture(t, s, factID)

	claimed, err := s.ClaimV1Facts(ctx, time.Now().UTC(), 100)
	if err != nil {
		t.Fatal(err)
	}
	fact, ok := findFact(claimed, factID)
	if !ok {
		t.Fatalf("fact %s was not claimed", factID)
	}
	wantOccurredAt := occurredAt.UTC().Truncate(time.Microsecond)
	if !fact.OccurredAt.Equal(wantOccurredAt) || fact.OccurredAt.Location() != time.UTC || fact.OccurredAt.Nanosecond() != wantOccurredAt.Nanosecond() {
		t.Fatalf("reloaded occurred_at=%s, want UTC microsecond %s", fact.OccurredAt, wantOccurredAt)
	}

	// Start from the same wire envelope used by the delivery worker, then model
	// the receiver's immutable-content check by omitting the digest field.
	delivered, err := json.Marshal(fact.Envelope())
	if err != nil {
		t.Fatal(err)
	}
	immutable := map[string]json.RawMessage{}
	if err := json.Unmarshal(delivered, &immutable); err != nil {
		t.Fatal(err)
	}
	delete(immutable, "immutable_content_sha256")
	canonical, err := canonicalV1JSON(immutable)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	if got := hex.EncodeToString(digest[:]); got != fact.ContentSHA256 {
		t.Fatalf("reloaded immutable digest=%s, stored=%s", got, fact.ContentSHA256)
	}
}

func TestV1FactOutboxReclaimsExpiredDeliveryLease(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL fact-outbox test")
	}
	ctx := context.Background()
	s, err := NewPostgresStore(config.Config{DatabaseURL: dsn})
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	factID := insertFactOutboxFixture(t, s, "lease-reclaim")
	defer deleteFactOutboxFixture(t, s, factID)

	now := time.Now().UTC()
	var beforePayload, beforeDigest string
	if err := s.db.QueryRowContext(ctx, `SELECT payload::text,content_digest FROM v1_fact_outbox WHERE fact_id=$1`, factID).Scan(&beforePayload, &beforeDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE v1_fact_outbox SET status='DELIVERING',claimed_until=$2,next_retry_at=$3 WHERE fact_id=$1`, factID, now.Add(-time.Second), time.Unix(0, 0).UTC()); err != nil {
		t.Fatal(err)
	}

	claimed, err := s.ClaimV1Facts(ctx, now, 100)
	if err != nil {
		t.Fatal(err)
	}
	fact, ok := findFact(claimed, factID)
	if !ok || fact.ContentSHA256 != beforeDigest || string(fact.Payload) != beforePayload {
		t.Fatalf("reclaimed fact=%#v present=%v, want immutable payload=%s digest=%s", fact, ok, beforePayload, beforeDigest)
	}
	if err := s.MarkV1FactDelivery(ctx, factID, 204, true, false, "", now); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM v1_fact_outbox WHERE fact_id=$1`, factID).Scan(&status); err != nil || status != "DELIVERED" {
		t.Fatalf("status=%q err=%v", status, err)
	}
}

func TestV1FactOutboxDoesNotStealActiveLeaseAndClaimsOnceConcurrently(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL fact-outbox test")
	}
	ctx := context.Background()
	s, err := NewPostgresStore(config.Config{DatabaseURL: dsn})
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	factID := insertFactOutboxFixture(t, s, "lease-contention")
	defer deleteFactOutboxFixture(t, s, factID)

	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `UPDATE v1_fact_outbox SET status='DELIVERING',claimed_until=$2 WHERE fact_id=$1`, factID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	activeClaims, err := s.ClaimV1Facts(ctx, now, 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, stolen := findFact(activeClaims, factID); stolen {
		t.Fatal("active fact-delivery lease was stolen")
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE v1_fact_outbox SET status='PENDING',claimed_until=NULL,next_retry_at=$2 WHERE fact_id=$1`, factID, time.Unix(0, 0).UTC()); err != nil {
		t.Fatal(err)
	}

	results := make(chan []V1Fact, 2)
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			facts, claimErr := s.ClaimV1Facts(ctx, now, 1)
			results <- facts
			errs <- claimErr
		}()
	}
	ready.Wait()
	count := 0
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		if _, claimed := findFact(<-results, factID); claimed {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("concurrent workers claimed fact %d times, want 1", count)
	}
}

func insertFactOutboxFixture(t *testing.T, s *PostgresStore, marker string) string {
	return insertFactOutboxFixtureAt(t, s, marker, time.Now().UTC())
}

func insertFactOutboxFixtureAt(t *testing.T, s *PostgresStore, marker string, occurredAt time.Time) string {
	t.Helper()
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	aggregate := "test-fact-" + NewUUIDString()
	if err := s.insertV1FactTx(ctx, tx, "charger.connection.updated", aggregate, nil, occurredAt, map[string]any{"marker": marker, "nested": map[string]any{"active": true}, "observed_at": occurredAt, "nullable": nil}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var factID string
	err = s.db.QueryRowContext(ctx, `SELECT fact_id::text FROM v1_fact_outbox WHERE aggregate_key=$1`, aggregate).Scan(&factID)
	if err != nil {
		t.Fatal(err)
	}
	return factID
}

func deleteFactOutboxFixture(t *testing.T, s *PostgresStore, factID string) {
	t.Helper()
	if _, err := s.db.ExecContext(context.Background(), `DELETE FROM v1_fact_outbox WHERE fact_id=$1`, factID); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
}

func findFact(facts []V1Fact, factID string) (V1Fact, bool) {
	for _, fact := range facts {
		if fact.FactID == factID {
			return fact, true
		}
	}
	return V1Fact{}, false
}
