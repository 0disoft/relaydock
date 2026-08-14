package main

/* llmnav/1 module
id=relaydock.command.mcp-bridge
role=Run the local MCP stdio bridge that delegates expert consultation tools to the RelayDock local IPC runtime.
owns=MCP bridge process assembly|stdio transport lifecycle|local IPC backend wiring
excludes=consultation execution|remote MCP authorization
search=run MCP stdio bridge|connect MCP to local runtime|expert bridge command
invariant=Stdout remains exclusive to MCP protocol traffic and diagnostics are written to stderr.
stability=architecture
*/

import (
	"context"
	"log/slog"
	"os"

	"github.com/0disoft/relaydock/internal/buildinfo"
	"github.com/0disoft/relaydock/internal/localipc"
	"github.com/0disoft/relaydock/internal/mcpbridge"
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
