package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewSecureUUIDStringReturnsCanonicalNonZeroUUID(t *testing.T) {
	id, err := NewSecureUUIDString()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := uuid.Parse(id)
	if err != nil || parsed == uuid.Nil || parsed.String() != id {
		t.Fatalf("id=%q parsed=%v err=%v", id, parsed, err)
	}
}

func TestIdentityGenerationFailureAbortsV1MemoryMutation(t *testing.T) {
	original := uuidGenerator
	uuidGenerator = func() (uuid.UUID, error) { return uuid.Nil, errors.New("entropy unavailable") }
	t.Cleanup(func() { uuidGenerator = original })

	store := NewV1MemoryStore()
	now := time.Now().UTC()
	_, _, err := store.CreateV1StartCommand(context.Background(), V1StartCommandInput{
		CMSCommandID: "command", RequestDigest: "digest", CPOID: "cpo", CMSStartIntentID: "intent", CMSChargerID: "charger", CMSConnectorID: "connector", ChargerOCPPIdentity: "CP-1", OCPPConnectorNumber: 1, IDTag: "appv1_entropy", CredentialExpiresAt: now.Add(time.Minute), CommandExpiresAt: now.Add(2 * time.Minute),
	})
	if err == nil {
		t.Fatal("expected UUID generation failure")
	}
	if _, err := store.GetV1Command(context.Background(), "command"); !errors.Is(err, ErrV1CommandNotFound) {
		t.Fatalf("partial command persisted: %v", err)
	}
	if _, err := store.GetV1Credential(context.Background(), "appv1_entropy"); !errors.Is(err, ErrV1CredentialRejected) {
		t.Fatalf("partial credential persisted: %v", err)
	}
}
