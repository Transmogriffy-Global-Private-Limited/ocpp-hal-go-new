package store

import (
	"fmt"
	mathrand "math/rand"

	"github.com/google/uuid"
)

const maxTransactionID = int64(2147483647)

var uuidGenerator = uuid.NewRandom

// NewSecureUUIDString returns a canonical non-zero UUIDv4 or the secure-randomness
// failure that prevented its creation. Callers that need a durable identity
// must abort before writing partial state when this returns an error.
func NewSecureUUIDString() (string, error) {
	id, err := uuidGenerator()
	if err != nil {
		return "", fmt.Errorf("generate UUID: %w", err)
	}
	if id == uuid.Nil {
		return "", fmt.Errorf("generate UUID: zero UUID")
	}
	return id.String(), nil
}

// MustNewUUIDString is test-fixture convenience only. Runtime code must use
// NewSecureUUIDString so a secure-randomness failure remains recoverable.
func MustNewUUIDString() string {
	id, err := NewSecureUUIDString()
	if err != nil {
		panic(err)
	}
	return id
}

// NewUUIDString is retained only for existing test fixtures. It never
// fabricates an identity: entropy failure panics rather than returning a
// timestamp- or math/rand-derived value. Runtime persistence uses
// NewSecureUUIDString directly.
func NewUUIDString() string { return MustNewUUIDString() }

func RandomTransactionID() int64 { return mathrand.Int63n(maxTransactionID) + 1 }

func DeltaWh(previous float64, current float64) float64 {
	const rollover = 4294967295.0
	delta := current - previous
	if delta >= 0 {
		return delta
	}
	return current + (rollover - previous) + 1
}

func floatPtr(v float64) *float64 { return &v }
