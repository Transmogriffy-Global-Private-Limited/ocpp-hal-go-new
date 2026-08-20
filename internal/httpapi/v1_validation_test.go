package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/store"
)

func TestValidUUIDRequiresCanonicalNonzeroUUID(t *testing.T) {
	valid := store.NewUUIDString()
	for _, value := range []string{valid, "00000000-0000-0000-0000-000000000000", strings.ToUpper(valid), valid + "0", "not-a-uuid"} {
		want := value == valid
		if got := validUUID(value); got != want {
			t.Fatalf("validUUID(%q)=%v, want %v", value, got, want)
		}
	}
}

func TestDecodeJSONRejectsTrailingValues(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"value":"first"} {"value":"second"}`))
	recorder := httptest.NewRecorder()
	var body struct {
		Value string `json:"value"`
	}
	if decodeJSON(recorder, request, &body) {
		t.Fatal("decodeJSON accepted a second JSON value")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", recorder.Code)
	}
}
