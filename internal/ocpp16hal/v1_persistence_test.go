package ocpp16hal

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/state"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

type failingV1CompletionStore struct{ store.V1Store }

func (failingV1CompletionStore) GetV1TransactionByOCPP(context.Context, string, int64) (*store.V1Transaction, error) {
	return &store.V1Transaction{HALTransactionID: store.NewUUIDString(), ChargerOCPPIdentity: "CP-persistence", OCPPConnectorNumber: 1, OCPPTransactionID: 9, ActualStartedAt: time.Now().UTC(), ObservedStartedAt: time.Now().UTC()}, nil
}

func (failingV1CompletionStore) CompleteV1Transaction(context.Context, string, int64, string, time.Time, time.Time) (*store.V1Transaction, error) {
	return nil, errors.New("database unavailable")
}

func TestOnStopTransactionReturnsErrorWhenPersistenceFails(t *testing.T) {
	h := New(state.NewRegistry(), failingV1CompletionStore{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	confirmation, err := h.OnStopTransaction("CP-persistence", &core.StopTransactionRequest{TransactionId: 9, MeterStop: 100})
	if err == nil || confirmation != nil {
		t.Fatalf("confirmation=%#v err=%v", confirmation, err)
	}
}
