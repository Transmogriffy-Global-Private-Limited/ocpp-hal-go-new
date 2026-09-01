package migrationrunner

import (
	"os"
	"path/filepath"
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
