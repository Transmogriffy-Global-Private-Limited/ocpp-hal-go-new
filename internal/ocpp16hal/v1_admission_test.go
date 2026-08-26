package ocpp16hal

import (
	"net/http/httptest"
	"testing"
)

func TestV1DeadlineStopCauseUsesThresholdProvenance(t *testing.T) {
	if initiator, reason := v1DeadlineStopCause("CUSTOMER_MONEY", "MONEY"); initiator != "MONEY_LIMIT" || reason != "money_limit_reached" {
		t.Fatalf("money deadline cause=%q/%q", initiator, reason)
	}
	if initiator, reason := v1DeadlineStopCause("WALLET", "TIME"); initiator != "WALLET_LIMIT" || reason != "wallet_limit_reached" {
		t.Fatalf("wallet deadline cause=%q/%q", initiator, reason)
	}
	if initiator, reason := v1DeadlineStopCause("CUSTOMER_TIME", "TIME"); initiator != "TIME_LIMIT" || reason != "time_limit_reached" {
		t.Fatalf("time deadline cause=%q/%q", initiator, reason)
	}
}

func TestParseChargerIdentityPathAcceptsOnlySupportedForms(t *testing.T) {
	for _, test := range []struct {
		name, path, wire, identity, serial string
		ok                                 bool
	}{
		{name: "identity", path: "/CP-1", wire: "CP-1", identity: "CP-1", ok: true},
		{name: "identity serial", path: "/CP-1/SN-1", wire: "SN-1", identity: "CP-1", serial: "SN-1", ok: true},
		{name: "empty", path: "/", wire: "", ok: false},
		{name: "extra segment", path: "/CP-1/SN-1/extra", wire: "extra", ok: false},
		{name: "double separator", path: "/CP-1//SN-1", wire: "SN-1", ok: false},
		{name: "wire mismatch", path: "/CP-1/SN-1", wire: "CP-1", ok: false},
		{name: "encoded separator", path: "/CP-1%2FSN-1", wire: "CP-1", ok: false},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "ws://example.test"+test.path, nil)
			identity, serial, ok := parseChargerIdentityPath(req, test.wire)
			if identity != test.identity || serial != test.serial || ok != test.ok {
				t.Fatalf("parse=(%q,%q,%v), want=(%q,%q,%v)", identity, serial, ok, test.identity, test.serial, test.ok)
			}
		})
	}
}

func TestWireIdentityAliasKeepsCanonicalIdentityAuthoritative(t *testing.T) {
	h := &HAL{wiredIdentity: make(map[string]string)}
	if !h.rememberWireIdentity("SN-1", "CP-1") {
		t.Fatal("initial alias rejected")
	}
	if got := h.canonicalIdentity("SN-1"); got != "CP-1" {
		t.Fatalf("canonical identity=%q", got)
	}
	if got := h.wireIdentityFor("CP-1"); got != "SN-1" {
		t.Fatalf("wire identity=%q", got)
	}
	if h.rememberWireIdentity("SN-1", "CP-2") {
		t.Fatal("ambiguous wire identity accepted")
	}
	h.forgetWireIdentity("SN-1")
	if got := h.canonicalIdentity("SN-1"); got != "SN-1" {
		t.Fatalf("forgotten alias canonical identity=%q", got)
	}
}
