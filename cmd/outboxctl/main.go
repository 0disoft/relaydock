package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/persistence/postgres"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	command := os.Args[1]
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	databaseURL := firstNonEmpty(os.Getenv("OUTBOX_POSTGRES_URL"), os.Getenv("ARG_POSTGRES_URL"), os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		fatalf("OUTBOX_POSTGRES_URL, ARG_POSTGRES_URL, or DATABASE_URL is required")
	}
	database, err := postgres.OpenSQL(ctx, databaseURL)
	if err != nil {
		fatalf("open PostgreSQL: %v", err)
	}
	defer database.Close()
	repository, err := postgres.NewOutboxRepository(database)
	if err != nil {
		fatalf("configure repository: %v", err)
	}

	switch command {
	case "status":
		status, err := repository.Status(ctx, time.Now().UTC())
		if err != nil {
			fatalf("query status: %v", err)
		}
		writeJSON(status)
	case "dead":
		set := flag.NewFlagSet("dead", flag.ExitOnError)
		limit := set.Int("limit", 100, "maximum dead letters to return (1-1000)")
		_ = set.Parse(os.Args[2:])
		records, err := repository.ListDeadLetters(ctx, *limit)
		if err != nil {
			fatalf("list dead letters: %v", err)
		}
		writeJSON(records)
	case "requeue":
		set := flag.NewFlagSet("requeue", flag.ExitOnError)
		id := set.String("id", "", "dead-letter event ID")
		delay := set.Duration("delay", 0, "delay before the event becomes available")
		reset := set.Bool("reset-attempts", false, "reset failed delivery attempts to zero")
		_ = set.Parse(os.Args[2:])
		if strings.TrimSpace(*id) == "" || *delay < 0 {
			fatalf("requeue requires --id and a non-negative --delay")
		}
		availableAt := time.Now().UTC().Add(*delay)
		if err := repository.RequeueDeadLetter(ctx, *id, availableAt, *reset); err != nil {
			fatalf("requeue dead letter: %v", err)
		}
		writeJSON(map[string]any{"id": *id, "availableAt": availableAt, "resetAttempts": *reset})
	case "purge-published":
		set := flag.NewFlagSet("purge-published", flag.ExitOnError)
		olderThan := set.Duration("older-than", 30*24*time.Hour, "purge events published before this age")
		limit := set.Int("limit", 10_000, "maximum rows to delete in this invocation (1-100000)")
		_ = set.Parse(os.Args[2:])
		if *olderThan <= 0 {
			fatalf("--older-than must be positive")
		}
		before := time.Now().UTC().Add(-*olderThan)
		count, err := repository.PurgePublishedBefore(ctx, before, *limit)
		if err != nil {
			fatalf("purge published events: %v", err)
		}
		writeJSON(map[string]any{"deleted": count, "before": before, "limit": *limit})
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  outboxctl status
  outboxctl dead [--limit 100]
  outboxctl requeue --id <event-id> [--delay 0s] [--reset-attempts]
  outboxctl purge-published [--older-than 720h] [--limit 10000]`)
}

func writeJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fatalf("encode output: %v", err)
	}
}

func fatalf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if strings.Contains(message, core.ErrConflict.Error()) {
		message += "; the event may already be owned, published, or requeued"
	}
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
