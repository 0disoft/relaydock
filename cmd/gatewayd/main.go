package main

/* llmnav/1 module
id=relaydock.command.gatewayd
role=Run the authenticated AI gateway with provider routing, accounting, leases, runtime journaling, and signed control-state synchronization.
owns=gateway process assembly|gateway dependency configuration|HTTP gateway lifecycle
excludes=provider protocol compilation|routing candidate policy
search=run AI gateway|configure provider gateway|gateway runtime service
invariant=Startup aborts when required database, secret, authentication, or runtime dependencies cannot be constructed safely.
stability=architecture
*/

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/0disoft/relaydock/internal/auth/virtualkey"
	"github.com/0disoft/relaydock/internal/buildinfo"
	gatewaycomposition "github.com/0disoft/relaydock/internal/composition/gateway"
	"github.com/0disoft/relaydock/internal/identifier"
	"github.com/0disoft/relaydock/internal/observability"
	"github.com/0disoft/relaydock/internal/persistence/postgres"
	valkeystore "github.com/0disoft/relaydock/internal/persistence/valkey"
	runtimegateway "github.com/0disoft/relaydock/internal/runtime"
	"github.com/0disoft/relaydock/internal/security/serversecrets"
	"github.com/0disoft/relaydock/internal/serverutil"
	"github.com/0disoft/relaydock/internal/transport/apiutil"
	"github.com/0disoft/relaydock/internal/transport/httpgateway"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := observability.NewLogger(os.Stdout, slog.LevelInfo).With("service", "gatewayd")
	address := envOr("GATEWAY_ADDRESS", "127.0.0.1:8080")
	token := strings.TrimSpace(os.Getenv("GATEWAY_BEARER_TOKEN"))
	database, err := openGatewayDatabase(ctx)
	if err != nil {
		logger.Error("open gateway database", "error", err)
		os.Exit(1)
	}
	if database != nil {
		defer database.Close()
	}
	secretResolver, err := serversecrets.NewDefaultResolver()
	if err != nil {
		logger.Error("configure server-secret resolver", "error", err)
		os.Exit(1)
	}
	authenticator, authMode, err := gatewayAuthenticator(ctx, database, secretResolver)
	if err != nil {
		logger.Error("configure gateway authentication", "error", err)
		os.Exit(1)
	}
	authenticationMarker := token
	if authenticator != nil && authenticationMarker == "" {
		authenticationMarker = "virtual-key-authentication"
	}
	if err := serverutil.RequireAuthenticationOutsideLoopback(address, authenticationMarker); err != nil {
		logger.Error("unsafe gateway configuration", "error", err)
		os.Exit(1)
	}
	runtime, err := gatewaycomposition.BuildFromEnvironmentWithCredentialSources(ctx, nil, secretResolver)
	if err != nil {
		logger.Error("construct provider runtime", "error", err)
		os.Exit(1)
	}
	journalMode, err := configureRuntimeJournal(runtime, database, authenticator != nil, token != "")
	if err != nil {
		logger.Error("configure runtime journal", "error", err)
		os.Exit(1)
	}
	leaseMode, closeRuntime, err := configureRuntime(runtime)
	if err != nil {
		logger.Error("configure gateway runtime", "error", err)
		os.Exit(1)
	}
	if closeRuntime != nil {
		defer closeRuntime()
	}
	snapshotManager, err := configureControlSnapshot(ctx, runtime, logger)
	if err != nil {
		logger.Error("configure control snapshot synchronization", "error", err)
		os.Exit(1)
	}
	handler := httpgateway.NewHandlerWithOptions(httpgateway.HandlerOptions{
		Processor:   runtime.Processor,
		Models:      runtime.Models,
		ModelSource: runtime.Candidates,
		Ready:       runtime.Candidates.Ready,
		RuntimeStatus: func(statusCtx context.Context) (map[string]any, error) {
			if err := statusCtx.Err(); err != nil {
				return nil, err
			}
			status := map[string]any{
				"version":        buildinfo.Version,
				"authentication": authMode,
				"providerLeases": leaseMode,
				"runtimeJournal": journalMode,
				"models":         runtime.Candidates.Models(),
				"routeSnapshot":  runtime.Candidates.SnapshotStatus(),
			}
			if snapshotManager != nil {
				status["controlSync"] = snapshotManager.Status()
			}
			return status, nil
		},
		Mode: "provider-runtime",
	})
	handler = apiutil.RequireGatewayAuthentication(token, authenticator, handler, "/healthz", "/readyz")
	snapshotMode := "static"
	var snapshotRevision int64
	if snapshotManager != nil {
		snapshotMode = "signed-control-plane"
		snapshotRevision = snapshotManager.Status().Revision
	}
	logger.Info(
		"starting gateway",
		"address", address,
		"authentication", authMode,
		"provider_leases", leaseMode,
		"runtime_journal", journalMode,
		"runtime_routes", snapshotMode,
		"snapshot_revision", snapshotRevision,
		"maximum_attempts", runtime.Gateway.MaximumAttempts,
		"lease_ttl", runtime.Gateway.LeaseTTL,
		"models", runtime.Candidates.Models(),
	)
	if err := serverutil.RunContext(ctx, address, handler, logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func configureRuntime(runtime *gatewaycomposition.Runtime) (string, func(), error) {
	if runtime == nil || runtime.Gateway == nil {
		return "", nil, fmt.Errorf("gateway runtime is not initialized")
	}
	maximumAttempts, err := intEnvironment("GATEWAY_MAX_ATTEMPTS", runtime.Gateway.MaximumAttempts, 1, 10)
	if err != nil {
		return "", nil, err
	}
	leaseTTL, err := durationEnvironment("GATEWAY_PROVIDER_LEASE_TTL", runtime.Gateway.LeaseTTL, time.Second, 24*time.Hour)
	if err != nil {
		return "", nil, err
	}
	retryBase, err := durationEnvironment("GATEWAY_RETRY_BASE_DELAY", runtime.Gateway.RetryBaseDelay, 0, time.Minute)
	if err != nil {
		return "", nil, err
	}
	retryMaximum, err := durationEnvironment("GATEWAY_RETRY_MAX_DELAY", runtime.Gateway.RetryMaximum, 0, 10*time.Minute)
	if err != nil {
		return "", nil, err
	}
	if retryMaximum > 0 && retryBase > retryMaximum {
		return "", nil, fmt.Errorf("GATEWAY_RETRY_BASE_DELAY must not exceed GATEWAY_RETRY_MAX_DELAY")
	}
	runtime.Gateway.MaximumAttempts = maximumAttempts
	runtime.Gateway.LeaseTTL = leaseTTL
	runtime.Gateway.RetryBaseDelay = retryBase
	runtime.Gateway.RetryMaximum = retryMaximum

	address := firstNonEmpty(os.Getenv("GATEWAY_VALKEY_URL"), os.Getenv("GATEWAY_VALKEY_ADDRESS"))
	if address == "" {
		return "memory", nil, nil
	}
	client, err := valkeystore.Open(address)
	if err != nil {
		return "", nil, err
	}
	closeClient := func() { client.Close() }
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		closeClient()
		return "", nil, err
	}
	manager, err := valkeystore.NewLeaseManager(client, envOr("GATEWAY_VALKEY_PREFIX", "arg"))
	if err != nil {
		closeClient()
		return "", nil, err
	}
	runtime.Gateway.Leases = manager
	return "valkey", closeClient, nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func openGatewayDatabase(ctx context.Context) (*sql.DB, error) {
	databaseURL := firstNonEmpty(os.Getenv("GATEWAY_POSTGRES_URL"), os.Getenv("ARG_POSTGRES_URL"))
	if databaseURL == "" {
		return nil, nil
	}
	openCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return postgres.OpenSQL(openCtx, databaseURL)
}

func gatewayAuthenticator(ctx context.Context, database *sql.DB, secretResolver virtualkey.SecretResolver) (virtualkey.Authenticator, string, error) {
	rawPepper := os.Getenv("GATEWAY_VIRTUAL_KEY_PEPPER")
	encodedPepper := os.Getenv("GATEWAY_VIRTUAL_KEY_PEPPER_B64")
	pepperReference := os.Getenv("GATEWAY_VIRTUAL_KEY_PEPPER_REF")
	if !virtualkey.PepperConfigured(rawPepper, encodedPepper, pepperReference) {
		if strings.TrimSpace(os.Getenv("GATEWAY_BEARER_TOKEN")) != "" {
			return nil, "operator-bearer", nil
		}
		return nil, "disabled", nil
	}
	if database == nil {
		return nil, "", fmt.Errorf("GATEWAY_POSTGRES_URL or ARG_POSTGRES_URL is required with virtual key authentication")
	}
	pepper, err := virtualkey.ResolvePepper(ctx, secretResolver, rawPepper, encodedPepper, pepperReference)
	if err != nil {
		return nil, "", err
	}
	defer clear(pepper)
	authenticator, err := virtualkey.NewPostgresAuthenticator(database, pepper, envOr("GATEWAY_KEY_ENVIRONMENT", "live"))
	if err != nil {
		return nil, "", err
	}
	return authenticator, "virtual-key", nil
}

func configureRuntimeJournal(runtime *gatewaycomposition.Runtime, database *sql.DB, virtualKeyAuthentication, operatorBearer bool) (string, error) {
	if runtime == nil {
		return "", fmt.Errorf("gateway runtime is not initialized")
	}
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("GATEWAY_RUNTIME_JOURNAL")))
	if mode == "" {
		if database != nil {
			mode = "postgres"
		} else {
			mode = "disabled"
		}
	}
	switch mode {
	case "disabled", "off", "none":
		runtime.Processor.Journal = nil
		return "disabled", nil
	case "memory":
		runtime.Processor.Journal = runtimegateway.NewMemoryJournal()
	case "postgres":
		if database == nil {
			return "", fmt.Errorf("PostgreSQL is required when GATEWAY_RUNTIME_JOURNAL=postgres")
		}
		journal, err := postgres.NewRuntimeJournal(database)
		if err != nil {
			return "", err
		}
		runtime.Processor.Journal = journal
	default:
		return "", fmt.Errorf("GATEWAY_RUNTIME_JOURNAL must be disabled, memory, or postgres")
	}
	runtime.Processor.PriceRevisionID = envOr("GATEWAY_PRICE_REVISION_ID", "unpriced")
	timeout, err := durationEnvironment("GATEWAY_RUNTIME_JOURNAL_TIMEOUT", 5*time.Second, 100*time.Millisecond, time.Minute)
	if err != nil {
		return "", err
	}
	runtime.Processor.JournalTimeout = timeout
	runtime.Processor.OperatorPrincipal = virtualkey.Principal{
		TenantID:     strings.TrimSpace(os.Getenv("GATEWAY_OPERATOR_TENANT_ID")),
		ProjectID:    strings.TrimSpace(os.Getenv("GATEWAY_OPERATOR_PROJECT_ID")),
		VirtualKeyID: strings.TrimSpace(os.Getenv("GATEWAY_OPERATOR_VIRTUAL_KEY_ID")),
	}
	if mode == "postgres" && (operatorBearer || !virtualKeyAuthentication) {
		principal := runtime.Processor.OperatorPrincipal
		if principal.TenantID == "" || principal.ProjectID == "" || principal.VirtualKeyID == "" {
			return "", fmt.Errorf("GATEWAY_OPERATOR_TENANT_ID, GATEWAY_OPERATOR_PROJECT_ID, and GATEWAY_OPERATOR_VIRTUAL_KEY_ID are required when PostgreSQL journaling can receive operator or unauthenticated traffic")
		}
		if !identifier.IsUUID(principal.ProjectID) || !identifier.IsUUID(principal.VirtualKeyID) {
			return "", fmt.Errorf("GATEWAY_OPERATOR_PROJECT_ID and GATEWAY_OPERATOR_VIRTUAL_KEY_ID must be canonical UUIDs")
		}
	}
	return mode, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func intEnvironment(name string, fallback, minimum, maximum int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minimum, maximum)
	}
	return value, nil
}

func durationEnvironment(name string, fallback, minimum, maximum time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be a duration between %s and %s", name, minimum, maximum)
	}
	return value, nil
}

func booleanEnvironment(name string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return value, nil
}

func int64Environment(name string, fallback, minimum, maximum int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minimum, maximum)
	}
	return value, nil
}
