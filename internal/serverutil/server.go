package serverutil

/* llmnav/1 module
id=relaydock.http.server-lifecycle
role=Run RelayDock HTTP handlers with bounded header reads, idle connections, signal cancellation, and graceful shutdown.
owns=HTTP server lifecycle|process signal shutdown|server timeout defaults
search=run RelayDock HTTP server|graceful shutdown timeout|server signal lifecycle
invariant=SIGINT or SIGTERM starts one graceful shutdown with a bounded fifteen-second budget.
invariant=http.ErrServerClosed is treated as an expected terminal state rather than a runtime failure.
risk=availability|concurrency
stability=architecture
*/

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

func Run(address string, handler http.Handler, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return RunContext(ctx, address, handler, logger)
}

func RunContext(ctx context.Context, address string, handler http.Handler, logger *slog.Logger) error {
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if logger != nil {
			logger.Info("shutting down")
		}
		return server.Shutdown(shutdownCtx)
	}
}
