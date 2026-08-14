package main

/* llmnav/1 module
id=relaydock.command.dbmigrate
role=Apply, inspect, or explicitly roll back the embedded RelayDock PostgreSQL migration sequence.
owns=database migration CLI|embedded migration execution|migration status output
excludes=migration definition authoring|runtime database queries
search=run database migrations|rollback migration steps|migration status
invariant=Migration execution uses the ordered embedded migration set and a caller-bounded context timeout.
stability=contract
*/

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	dbassets "github.com/0disoft/relaydock/db"
	"github.com/0disoft/relaydock/internal/persistence/migrate"
	"github.com/0disoft/relaydock/internal/persistence/postgres"
)

func main() {
	var (
		databaseURL = flag.String("database-url", firstNonEmpty(os.Getenv("ARG_POSTGRES_URL"), os.Getenv("DATABASE_URL")), "PostgreSQL connection URL")
		timeout     = flag.Duration("timeout", 5*time.Minute, "overall migration timeout")
		steps       = flag.Int("steps", 1, "number of migrations to roll back with down")
	)
	flag.Usage = func() {
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), "Usage: dbmigrate [flags] <up|down|status>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *databaseURL == "" {
		exitf("database URL is required through --database-url, ARG_POSTGRES_URL, or DATABASE_URL")
	}
	command := "up"
	if flag.NArg() > 0 {
		command = strings.ToLower(strings.TrimSpace(flag.Arg(0)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	database, err := postgres.OpenSQL(ctx, *databaseURL)
	if err != nil {
		exitf("open PostgreSQL: %v", err)
	}
	defer database.Close()
	migrations, err := migrate.Load(dbassets.Migrations, "migrations")
	if err != nil {
		exitf("load migrations: %v", err)
	}
	runner, err := migrate.New(database, migrations)
	if err != nil {
		exitf("construct migration runner: %v", err)
	}
	switch command {
	case "up":
		applied, err := runner.Up(ctx)
		if err != nil {
			exitf("migrate up: %v", err)
		}
		if len(applied) == 0 {
			fmt.Println("database is already current")
			return
		}
		for _, value := range applied {
			fmt.Printf("applied %06d_%s\n", value.Version, value.Name)
		}
	case "down":
		rolledBack, err := runner.Down(ctx, *steps)
		if err != nil {
			exitf("migrate down: %v", err)
		}
		if len(rolledBack) == 0 {
			fmt.Println("database has no applied migrations")
			return
		}
		for _, value := range rolledBack {
			fmt.Printf("rolled back %06d_%s\n", value.Version, value.Name)
		}
	case "status":
		statuses, err := runner.Status(ctx)
		if err != nil {
			exitf("migration status: %v", err)
		}
		for _, status := range statuses {
			state := "pending"
			if status.Applied != nil {
				state = "applied " + status.Applied.AppliedAt.UTC().Format(time.RFC3339)
			}
			fmt.Printf("%06d_%s\t%s\n", status.Migration.Version, status.Migration.Name, state)
		}
	default:
		flag.Usage()
		exitf("unknown command %q", command)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func exitf(format string, arguments ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", arguments...)
	os.Exit(1)
}
