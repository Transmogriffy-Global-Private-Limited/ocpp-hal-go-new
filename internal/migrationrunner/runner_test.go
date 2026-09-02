package migrationrunner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreatedTablesFindsApplicationRelationsOnlyOnce(t *testing.T) {
	got := CreatedTables(`CREATE TABLE public.first_table (id int); CREATE TABLE IF NOT EXISTS second_table (id int); CREATE TABLE first_table (id int);`)
	if len(got) != 2 || got[0] != "first_table" || got[1] != "second_table" {
		t.Fatalf("tables=%#v", got)
	}
}

func TestTraceMigrationCreatesOwnershipCheckedRelations(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "018_add_v1_charging_trace.sql"))
	if err != nil {
		t.Fatal(err)
	}
	got := CreatedTables(string(raw))
	if len(got) != 2 || got[0] != "v1_charging_traces" || got[1] != "v1_charging_trace_events" {
		t.Fatalf("trace migration ownership checks would cover %#v", got)
	}
}

func TestTraceDeliveryOutboxMigrationIsAdditiveAndIndependent(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "019_add_v1_trace_delivery_outbox.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, want := range []string{
		"CREATE TABLE v1_trace_delivery_outbox",
		"REFERENCES v1_charging_trace_events(event_id)",
		"ix_v1_trace_delivery_outbox_due",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("trace delivery migration missing %q", want)
		}
	}
	if strings.Contains(sql, "ALTER TABLE v1_fact_outbox") || strings.Contains(sql, "DROP TABLE v1_fact_outbox") {
		t.Fatal("diagnostic trace delivery must not alter authoritative fact delivery")
	}
}
