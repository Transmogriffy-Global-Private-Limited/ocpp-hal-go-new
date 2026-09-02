package store

import (
	"strings"
	"testing"
)

func TestMarkV1TraceDeliverySQLTypesEveryStatusUse(t *testing.T) {
	const typedStatus = "$2::varchar(32)"
	if got := strings.Count(markV1TraceDeliverySQL, typedStatus); got != 3 {
		t.Fatalf("typed status uses=%d, want 3 in %s", got, markV1TraceDeliverySQL)
	}
	if strings.Contains(markV1TraceDeliverySQL, "status=$2,") || strings.Contains(markV1TraceDeliverySQL, "WHEN $2='") {
		t.Fatalf("mark SQL retains an untyped status parameter: %s", markV1TraceDeliverySQL)
	}
}

func TestMarkV1TraceDeliverySQLPreservesTerminalSemantics(t *testing.T) {
	for _, want := range []string{
		"status=$2::varchar(32)",
		"CASE WHEN $2::varchar(32)='DELIVERED' THEN retries ELSE retries+1 END",
		"claimed_until=NULL,claim_token=NULL",
		"delivery_status_code=$4,last_error=$5",
		"CASE WHEN $2::varchar(32)='DELIVERED' THEN NOW() ELSE sent_at END",
		"status='DELIVERING' AND claim_token=$6::uuid",
	} {
		if !strings.Contains(markV1TraceDeliverySQL, want) {
			t.Fatalf("mark SQL missing %q: %s", want, markV1TraceDeliverySQL)
		}
	}
}

func TestV1TraceDeliveryStatusPreservesOutcomeSemantics(t *testing.T) {
	for _, testcase := range []struct {
		name              string
		success, terminal bool
		want              string
	}{
		{name: "delivered", success: true, want: "DELIVERED"},
		{name: "retry", want: "RETRY"},
		{name: "reconciliation required", terminal: true, want: "RECONCILIATION_REQUIRED"},
		{name: "terminal dominates success", success: true, terminal: true, want: "RECONCILIATION_REQUIRED"},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			if got := v1TraceDeliveryStatus(testcase.success, testcase.terminal); got != testcase.want {
				t.Fatalf("status=%q want %q", got, testcase.want)
			}
		})
	}
}
