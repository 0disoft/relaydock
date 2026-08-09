package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/your-org/ai-runtime-gateway/internal/buildinfo"
	"github.com/your-org/ai-runtime-gateway/internal/localipc"
	"github.com/your-org/ai-runtime-gateway/internal/mcpbridge"
)

func main() {
	// MCP reserves stdout for JSON-RPC. All diagnostics go to stderr.
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	client := localipc.NewClient(localipc.DefaultEndpoint())
	server := mcpbridge.NewServer(mcpbridge.NewIPCBackend(client), buildinfo.Version)

	if err := server.RunStdio(context.Background()); err != nil {
		logger.Error("mcp bridge stopped", "error", err)
		os.Exit(1)
	}
}
