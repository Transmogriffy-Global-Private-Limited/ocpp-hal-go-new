// Package migrationrunner owns the guarded application-DDL execution path.
package migrationrunner

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

var createTable = regexp.MustCompile(`(?im)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:[a-z_][a-z0-9_]*\.)?([a-z_][a-z0-9_]*)`)

func CreatedTables(sqlText string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, match := range createTable.FindAllStringSubmatch(sqlText, -1) {
		name := strings.ToLower(match[1])
		if _, exists := seen[name]; !exists {
			seen[name] = struct{}{}
			result = append(result, name)
		}
	}
	return result
}

// Apply executes one reviewed migration as the configured application/schema
// role. The connection may be privileged, but the DDL itself is not. Role and
// table-owner checks fail the transaction before commit on any drift.
func Apply(ctx context.Context, db *sql.DB, applicationRole, sqlText string) error {
	role := strings.TrimSpace(applicationRole)
	if role == "" {
		return fmt.Errorf("HAL_MIGRATION_APPLICATION_ROLE is required")
	}
	if strings.TrimSpace(sqlText) == "" {
		return fmt.Errorf("migration SQL is empty")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var quotedRole string
	if err := tx.QueryRowContext(ctx, `SELECT quote_ident($1)`, role).Scan(&quotedRole); err != nil {
		return fmt.Errorf("quote configured application role: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "SET LOCAL ROLE "+quotedRole); err != nil {
		return fmt.Errorf("set configured application role: %w", err)
	}
	var currentUser string
	if err := tx.QueryRowContext(ctx, `SELECT current_user`).Scan(&currentUser); err != nil {
		return fmt.Errorf("verify migration current_user: %w", err)
	}
	if currentUser != role {
		return fmt.Errorf("migration current_user %q does not match configured application role %q", currentUser, role)
	}
	if _, err := tx.ExecContext(ctx, sqlText); err != nil {
		return fmt.Errorf("execute application migration DDL: %w", err)
	}
	// RESET ROLE before ownership checks proves the guard did not leave the
	// privileged migration session impersonating the runtime application role.
	if _, err := tx.ExecContext(ctx, `RESET ROLE`); err != nil {
		return fmt.Errorf("reset migration role: %w", err)
	}
	for _, table := range CreatedTables(sqlText) {
		var owner string
		err := tx.QueryRowContext(ctx, `SELECT pg_get_userbyid(c.relowner) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname=$1 AND c.relkind IN ('r','p')`, table).Scan(&owner)
		if err != nil {
			return fmt.Errorf("inspect migration relation owner for %s: %w", table, err)
		}
		if owner != role {
			return fmt.Errorf("migration relation %s owner %q does not match configured application role %q", table, owner, role)
		}
	}
	return tx.Commit()
}
