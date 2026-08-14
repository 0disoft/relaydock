package main

/* llmnav/1 module
id=relaydock.command.expertstorectl
role=Inspect, verify, and compact the local expert consultation store with explicit retention and dry-run controls.
owns=expert store maintenance CLI|store statistics output|bounded consultation compaction
excludes=consultation execution|remote persistence administration
search=compact expert store|verify consultation state|expert storage statistics
invariant=Dry-run compaction reports the proposed changes without mutating local consultation state.
stability=contract
*/

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/appdirs"
	"github.com/0disoft/relaydock/internal/expert/localstore"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "stats":
		err = runStats(os.Args[2:])
	case "compact":
		err = runCompact(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "expertstorectl:", err)
		os.Exit(1)
	}
}

func runStats(arguments []string) error {
	flags := flag.NewFlagSet("stats", flag.ContinueOnError)
	state := flags.String("state", defaultStatePath(), "local Expert state path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	store, err := localstore.Open(*state)
	if err != nil {
		return err
	}
	stats, err := store.Stats(context.Background())
	if err != nil {
		return err
	}
	return writeJSON(map[string]any{"path": store.Path(), "stats": stats})
}

func runCompact(arguments []string) error {
	flags := flag.NewFlagSet("compact", flag.ContinueOnError)
	state := flags.String("state", defaultStatePath(), "local Expert state path")
	terminalRetention := flags.Duration("terminal-retention", 30*24*time.Hour, "retention for completed, failed, cancelled, and expired consultations")
	orphanGrace := flags.Duration("orphan-grace", 24*time.Hour, "minimum age before unreferenced packs, results, and chunks are removed")
	removeOrphans := flags.Bool("remove-orphans", true, "remove unreferenced ContextPacks and results")
	dryRun := flags.Bool("dry-run", false, "report changes without mutating state")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	store, err := localstore.Open(*state)
	if err != nil {
		return err
	}
	before, err := store.Stats(context.Background())
	if err != nil {
		return err
	}
	report, err := store.Compact(context.Background(), localstore.CompactPolicy{
		TerminalRetention: *terminalRetention,
		OrphanGracePeriod: *orphanGrace,
		RemoveOrphans:     *removeOrphans,
		DryRun:            *dryRun,
	})
	if err != nil {
		return err
	}
	after := before
	if !*dryRun {
		after, err = store.Stats(context.Background())
		if err != nil {
			return err
		}
	}
	return writeJSON(map[string]any{"path": store.Path(), "dryRun": *dryRun, "before": before, "report": report, "after": after})
}

func runVerify(arguments []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	state := flags.String("state", defaultStatePath(), "local Expert state path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	store, err := localstore.Open(*state)
	if err != nil {
		return err
	}
	stats, err := store.Stats(context.Background())
	if err != nil {
		return err
	}
	return writeJSON(map[string]any{"path": store.Path(), "valid": true, "stats": stats})
}

func defaultStatePath() string {
	if configured := strings.TrimSpace(os.Getenv("EXPERT_STATE_PATH")); configured != "" {
		return configured
	}
	config, err := appdirs.ConfigDir("")
	if err != nil {
		return filepath.Join(".", "expert-state.json")
	}
	return filepath.Join(config, "expert-state.json")
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: expertstorectl <stats|verify|compact> [flags]")
}
