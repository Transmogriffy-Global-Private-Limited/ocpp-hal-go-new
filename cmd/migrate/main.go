// Command migrate is the only approved source-tree path for applying HAL DDL.
// It deliberately requires an explicit reviewed migration file and configured
// application/schema role; it never guesses a deployment role.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/migrationrunner"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	file := flag.String("file", "", "reviewed migration SQL file to apply")
	flag.Parse()
	if strings.TrimSpace(*file) == "" {
		fmt.Fprintln(os.Stderr, "-file is required")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid configuration:", err)
		os.Exit(2)
	}
	sqlText, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read migration:", err)
		os.Exit(2)
	}
	db, err := sql.Open("pgx", cfg.PostgresURL())
	if err != nil {
		fmt.Fprintln(os.Stderr, "open migration database:", err)
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "ping migration database:", err)
		os.Exit(1)
	}
	if err := migrationrunner.Apply(ctx, db, cfg.MigrationApplicationRole, string(sqlText)); err != nil {
		fmt.Fprintln(os.Stderr, "migration failed:", err)
		os.Exit(1)
	}
	fmt.Println("migration applied under configured application role")
}
