package main

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
