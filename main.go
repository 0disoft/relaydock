package main

import (
	"embed"
	"log/slog"
	"os"

	"github.com/your-org/ai-runtime-gateway/internal/desktopwails"
)

// frontendAssets contains the production Svelte bundle.
//
//go:embed all:frontend/dist
var frontendAssets embed.FS

func main() {
	if err := desktopwails.Run(frontendAssets); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("desktop application stopped", "error", err)
		os.Exit(1)
	}
}
