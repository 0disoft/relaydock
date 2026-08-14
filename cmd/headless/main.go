package main

/* llmnav/1 module
id=relaydock.command.headless
role=Run the persistent local RelayDock runtime without a desktop UI and expose it through the platform-local IPC endpoint.
owns=headless runtime process|persistent local runtime assembly|local IPC server lifecycle
excludes=MCP tool registration|remote HTTP serving
search=headless local runtime|RelayDock local IPC|run without desktop UI
invariant=The headless process exposes the runtime only through the configured local IPC transport.
stability=architecture
*/

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/0disoft/relaydock/internal/localipc"
	"github.com/0disoft/relaydock/internal/localruntime"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "headless")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runtime, err := localruntime.NewPersistent(os.Getenv("ARG_DESKTOP_DATA_DIR"), os.Getenv("ARG_DEFAULT_REPOSITORY_ROOT"))
	if err != nil {
		logger.Error("construct persistent runtime", "error", err)
		os.Exit(1)
	}
	endpoint := localipc.DefaultEndpoint()
	server := localipc.NewServer(endpoint, runtime.Handler())
	logger.Info("headless runtime listening", "endpoint", endpoint)
	if err := server.Run(ctx); err != nil {
		logger.Error("headless runtime stopped", "error", err)
		os.Exit(1)
	}
}
