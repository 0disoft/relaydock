package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/releasepack"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "audit":
		err = runAudit(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	case "build":
		err = runBuild(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runAudit(arguments []string) error {
	flags := flag.NewFlagSet("audit", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	maximum := flags.Int64("max-bytes", releasepack.DefaultMaximumFileBytes, "maximum regular file size")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	report, err := releasepack.Audit(*root, *maximum)
	if err != nil {
		return err
	}
	fmt.Printf("audit ok: files=%d totalBytes=%d largest=%s (%d bytes)\n", report.Files, report.TotalBytes, report.LargestFilePath, report.LargestFileBytes)
	return nil
}

func runVerify(arguments []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	report, err := releasepack.Verify(*root)
	if err != nil {
		return err
	}
	fmt.Printf("manifest ok: files=%d chunks=%d totalBytes=%d largest=%s (%d bytes)\n", report.Files, report.ManifestChunks, report.TotalBytes, report.LargestFilePath, report.LargestFileBytes)
	return nil
}

func runBuild(arguments []string) error {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	output := flags.String("output", "", "output ZIP path")
	prefix := flags.String("prefix", "ai-runtime-gateway", "archive root directory")
	generated := flags.String("generated-at", "", "RFC3339 archive timestamp")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *output == "" {
		version, err := os.ReadFile(filepath.Join(*root, "VERSION"))
		if err != nil {
			return fmt.Errorf("read VERSION: %w", err)
		}
		*output = filepath.Join(filepath.Dir(filepath.Clean(*root)), "ai-runtime-gateway-"+stringTrimSpace(version)+".zip")
	}
	var generatedAt time.Time
	var err error
	if *generated != "" {
		generatedAt, err = time.Parse(time.RFC3339, *generated)
		if err != nil {
			return fmt.Errorf("parse generated-at: %w", err)
		}
	}
	report, err := releasepack.Build(releasepack.BuildOptions{
		Root: *root, OutputZIP: *output, ArchivePrefix: *prefix, GeneratedAt: generatedAt,
	})
	if err != nil {
		return err
	}
	fmt.Printf("archive built: %s\nsha256: %s\nfiles: %d manifestChunks: %d bytes: %d\n", report.OutputZIP, report.ArchiveSHA256, report.Files, report.ManifestChunks, report.ArchiveBytes)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: releasepack <audit|verify|build> [flags]")
}

func stringTrimSpace(value []byte) string {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\n' || value[start] == '\r' || value[start] == '\t') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\n' || value[end-1] == '\r' || value[end-1] == '\t') {
		end--
	}
	return string(value[start:end])
}
