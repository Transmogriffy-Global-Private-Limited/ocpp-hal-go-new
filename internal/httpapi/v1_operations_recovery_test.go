package httpapi

import (
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

func TestShouldDispatchV1ChargerOperation(t *testing.T) {
	tests := []struct {
		name      string
		duplicate bool
		state     string
		want      bool
	}{
		{name: "new operation", want: true},
		{name: "persisted duplicate retries safe pre-network claim", duplicate: true, state: "PERSISTED", want: true},
		{name: "attempted duplicate never resends", duplicate: true, state: "DELIVERY_ATTEMPTED", want: false},
		{name: "confirmed duplicate never resends", duplicate: true, state: "OCPP_CONFIRMED", want: false},
		{name: "reconciliation duplicate never resends", duplicate: true, state: "RECONCILIATION_REQUIRED", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldDispatchV1ChargerOperation(test.duplicate, &store.V1ChargerOperation{State: test.state}); got != test.want {
				t.Fatalf("dispatch=%t, want %t", got, test.want)
			}
		})
	}
}
