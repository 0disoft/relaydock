package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	gatewaycomposition "github.com/0disoft/relaydock/internal/composition/gateway"
	"github.com/0disoft/relaydock/internal/control/snapshot"
)

const defaultGatewaySnapshotPath = "state/gateway-runtime-snapshot.json"

// configureControlSnapshot connects gatewayd to controld's signed runtime
// snapshot stream. Static routes remain available when synchronization is not
// configured. Once GATEWAY_CONTROL_URL is present, freshness is required by
// default so an expired policy cannot silently keep serving traffic.
func configureControlSnapshot(
	ctx context.Context,
	runtime *gatewaycomposition.Runtime,
	logger *slog.Logger,
) (*snapshot.Manager, error) {
	if runtime == nil || runtime.Candidates == nil {
		return nil, fmt.Errorf("gateway candidate source is not initialized")
	}
	baseURL := strings.TrimSpace(os.Getenv("GATEWAY_CONTROL_URL"))
	required, err := booleanEnvironment("GATEWAY_CONTROL_REQUIRED", baseURL != "")
	if err != nil {
		return nil, err
	}
	allowDirect, err := booleanEnvironment("GATEWAY_ALLOW_DIRECT_MODELS", baseURL == "")
	if err != nil {
		return nil, err
	}
	runtime.Candidates.SetAllowDirectRouting(allowDirect)
	if baseURL == "" {
		if required {
			return nil, fmt.Errorf("GATEWAY_CONTROL_URL is required when GATEWAY_CONTROL_REQUIRED=true")
		}
		runtime.Candidates.SetRequireFreshSnapshot(false)
		return nil, nil
	}

	verifier, trustedKeyIDs, err := loadControlVerifier()
	if err != nil {
		return nil, err
	}
	client, err := snapshot.NewRemoteClientWithVerifier(
		baseURL,
		strings.TrimSpace(os.Getenv("GATEWAY_CONTROL_BEARER_TOKEN")),
		verifier,
		&http.Client{Transport: http.DefaultTransport},
	)
	if err != nil {
		return nil, err
	}
	maximumSize, err := int64Environment("GATEWAY_CONTROL_MAX_SNAPSHOT_BYTES", 32<<20, 64<<10, 256<<20)
	if err != nil {
		return nil, err
	}
	client.MaximumSnapshotSize = maximumSize
	client.MaximumFutureSkew, err = durationEnvironment("GATEWAY_CONTROL_MAX_FUTURE_SKEW", 5*time.Minute, 0, time.Hour)
	if err != nil {
		return nil, err
	}

	lastKnown, err := snapshot.OpenLocalStore(envOr("GATEWAY_CONTROL_LKG_PATH", defaultGatewaySnapshotPath))
	if err != nil {
		return nil, fmt.Errorf("open gateway last-known-good snapshot: %w", err)
	}
	manager := &snapshot.Manager{
		Client:       client,
		LastKnown:    lastKnown,
		Apply:        runtime.Candidates.ApplySnapshot,
		Required:     required,
		FetchTimeout: 15 * time.Second,
		BackoffMin:   time.Second,
		BackoffMax:   30 * time.Second,
		Logger:       logger,
	}
	runtime.Candidates.SetRequireFreshSnapshot(required)
	if logger != nil {
		logger.InfoContext(ctx, "configured control snapshot trust ring", "keyIds", trustedKeyIDs)
	}
	if err := manager.Bootstrap(ctx); err != nil {
		return nil, err
	}
	go func() {
		if err := manager.Run(ctx); err != nil && ctx.Err() == nil && logger != nil {
			logger.ErrorContext(ctx, "runtime snapshot synchronization stopped", "error", err)
		}
	}()
	return manager, nil
}
