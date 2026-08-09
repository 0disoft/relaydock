package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/auth/mcpconfig"
	"github.com/your-org/ai-runtime-gateway/internal/auth/scopedtoken"
	"github.com/your-org/ai-runtime-gateway/internal/buildinfo"
	"github.com/your-org/ai-runtime-gateway/internal/core"
	expertapp "github.com/your-org/ai-runtime-gateway/internal/expert/app"
	"github.com/your-org/ai-runtime-gateway/internal/expert/routes/openaiapi"
	"github.com/your-org/ai-runtime-gateway/internal/expert/worker"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
	"github.com/your-org/ai-runtime-gateway/internal/mcpremote"
	"github.com/your-org/ai-runtime-gateway/internal/observability"
	"github.com/your-org/ai-runtime-gateway/internal/persistence/postgres"
	"github.com/your-org/ai-runtime-gateway/internal/serverutil"
	"github.com/your-org/ai-runtime-gateway/internal/transport/apiutil"
	"github.com/your-org/ai-runtime-gateway/internal/transport/experthttp"
)

func main() {
	logger := observability.NewLogger(os.Stdout, slog.LevelInfo).With("service", "expert-brokerd")
	statePath := strings.TrimSpace(os.Getenv("EXPERT_STATE_PATH"))
	if statePath == "" {
		statePath = filepath.Join("data", "expert-broker", "state.json")
	}
	storageMode := "local-file"
	storageDescription := statePath
	var closeStorage func()
	var application *expertapp.App
	var err error
	databaseURL := firstNonEmpty(os.Getenv("EXPERT_POSTGRES_URL"), os.Getenv("ARG_POSTGRES_URL"))
	if databaseURL != "" {
		startupCtx, cancelStartup := context.WithTimeout(context.Background(), 15*time.Second)
		database, openErr := postgres.OpenSQL(startupCtx, databaseURL)
		cancelStartup()
		if openErr != nil {
			logger.Error("open expert PostgreSQL storage", "error", openErr)
			os.Exit(1)
		}
		closeStorage = func() { _ = database.Close() }
		application, err = expertapp.NewPostgres(
			database,
			strings.TrimSpace(os.Getenv("EXPERT_TENANT_ID")),
			strings.TrimSpace(os.Getenv("EXPERT_PROJECT_ID")),
		)
		storageMode = "postgres"
		storageDescription = "managed"
	} else {
		application, err = expertapp.NewLocal(statePath)
	}
	if err != nil {
		if closeStorage != nil {
			closeStorage()
		}
		logger.Error("initialise expert application", "error", err)
		os.Exit(1)
	}
	if closeStorage != nil {
		defer closeStorage()
	}

	address := envOr("EXPERT_BROKER_ADDRESS", "127.0.0.1:8082")
	brokerToken := strings.TrimSpace(os.Getenv("EXPERT_BROKER_BEARER_TOKEN"))
	mcpTokenVerifier, mcpAuthentication, err := expertMCPTokenVerifier()
	if err != nil {
		logger.Error("configure scoped MCP tokens", "error", err)
		os.Exit(1)
	}
	mcpStaticToken, err := mcpconfig.StaticToken(
		os.Getenv("EXPERT_MCP_BEARER_TOKEN"),
		brokerToken,
		os.Getenv("EXPERT_MCP_ALLOW_BROKER_TOKEN"),
		mcpTokenVerifier != nil,
	)
	if err != nil {
		logger.Error("configure MCP legacy authentication", "error", err)
		os.Exit(1)
	}
	if err := serverutil.RequireAuthenticationOutsideLoopback(address, brokerToken); err != nil {
		logger.Error("unsafe broker configuration", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/", experthttp.NewHandlerWithApp(application))
	if mcpStaticToken != "" || mcpTokenVerifier != nil {
		mux.Handle("/mcp", mcpremote.NewHandlerWithOptions(
			mcpremote.NewServiceBackend(application),
			buildinfo.Version,
			mcpremote.HandlerOptions{
				BearerToken:    mcpStaticToken,
				TokenVerifier:  mcpTokenVerifier,
				AllowedHosts:   splitCSV(os.Getenv("EXPERT_MCP_ALLOWED_HOSTS")),
				AllowedOrigins: splitCSV(os.Getenv("EXPERT_MCP_ALLOWED_ORIGINS")),
				Logger:         logger,
			},
		))
		logger.Info("remote MCP endpoint enabled", "path", "/mcp", "authentication", mcpAuthentication, "legacyStaticToken", mcpStaticToken != "")
	} else {
		logger.Info("remote MCP endpoint disabled", "reason", "broker bearer token is empty")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	workerDone := make(chan error, 1)
	workerEnabled, err := startExpertWorker(ctx, application, logger, workerDone)
	if err != nil {
		logger.Error("configure expert worker", "error", err)
		os.Exit(1)
	}

	handler := apiutil.RequireBearer(brokerToken, mux, "/healthz", "/readyz", "/mcp")
	logger.Info("starting expert broker", "address", address, "version", buildinfo.Version, "authentication", brokerToken != "", "storage", storageMode, "storageDescription", storageDescription, "workerEnabled", workerEnabled)
	serverErr := serverutil.RunContext(ctx, address, handler, logger)
	stop()
	if workerEnabled {
		select {
		case workerErr := <-workerDone:
			if workerErr != nil {
				logger.Error("expert worker stopped", "error", workerErr)
			}
		case <-time.After(15 * time.Second):
			logger.Error("expert worker shutdown timed out")
		}
	}
	if serverErr != nil {
		logger.Error("server stopped", "error", serverErr)
		os.Exit(1)
	}
}

func expertMCPTokenVerifier() (*scopedtoken.Service, string, error) {
	rawSecret := strings.TrimSpace(os.Getenv("EXPERT_MCP_TOKEN_SECRET"))
	encodedSecret := strings.TrimSpace(os.Getenv("EXPERT_MCP_TOKEN_SECRET_B64"))
	if rawSecret == "" && encodedSecret == "" {
		return nil, "legacy-static", nil
	}
	var secret []byte
	if encodedSecret != "" {
		decoded, err := scopedtoken.DecodeBase64Secret(encodedSecret)
		if err != nil {
			return nil, "", fmt.Errorf("%w: decode EXPERT_MCP_TOKEN_SECRET_B64", core.ErrInvalidConfiguration)
		}
		secret = decoded
	} else {
		secret = []byte(rawSecret)
	}
	keyID := strings.TrimSpace(os.Getenv("EXPERT_MCP_TOKEN_KEY_ID"))
	if keyID == "" {
		keyID = scopedtoken.KeyID(secret)
	}
	verificationKeys, err := scopedtoken.ParseBase64KeyMap(os.Getenv("EXPERT_MCP_TOKEN_VERIFICATION_KEYS_B64"))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", core.ErrInvalidConfiguration, err)
	}
	service, err := scopedtoken.NewKeyRing(keyID, secret, verificationKeys, envOr("EXPERT_MCP_TOKEN_AUDIENCE", "expert-mcp"))
	if err != nil {
		return nil, "", err
	}
	maximumLifetime, err := parseDuration(
		"EXPERT_MCP_TOKEN_MAX_LIFETIME",
		os.Getenv("EXPERT_MCP_TOKEN_MAX_LIFETIME"),
		24*time.Hour,
		time.Minute,
		30*24*time.Hour,
	)
	if err != nil {
		return nil, "", err
	}
	allowLegacy := true
	if raw := strings.TrimSpace(os.Getenv("EXPERT_MCP_TOKEN_ALLOW_LEGACY")); raw != "" {
		allowLegacy, err = strconv.ParseBool(raw)
		if err != nil {
			return nil, "", fmt.Errorf("%w: EXPERT_MCP_TOKEN_ALLOW_LEGACY must be true or false", core.ErrInvalidConfiguration)
		}
	}
	return service.WithMaximumLifetime(maximumLifetime).WithLegacyVerification(allowLegacy), "scoped-hmac-keyring", nil
}

func startExpertWorker(ctx context.Context, application *expertapp.App, logger *slog.Logger, done chan<- error) (bool, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	model := strings.TrimSpace(os.Getenv("EXPERT_MODEL"))
	if apiKey == "" || model == "" {
		logger.Info("expert API worker disabled", "reason", "OPENAI_API_KEY and EXPERT_MODEL are both required")
		return false, nil
	}
	client := openaiapi.New(strings.TrimSpace(os.Getenv("OPENAI_RESPONSES_ENDPOINT")), apiKey, nil)
	pricing, err := pricingFromEnvironment()
	if err != nil {
		return false, err
	}
	if pricing != nil {
		client = client.WithPricing(*pricing)
	}
	maximumOutputTokens, err := parseNonNegativeInt64("EXPERT_MAX_OUTPUT_TOKENS", os.Getenv("EXPERT_MAX_OUTPUT_TOKENS"))
	if err != nil {
		return false, err
	}
	concurrency, err := parsePositiveInt("EXPERT_WORKER_CONCURRENCY", os.Getenv("EXPERT_WORKER_CONCURRENCY"), 1, 64)
	if err != nil {
		return false, err
	}
	maximumAttempts, err := parsePositiveInt("EXPERT_WORKER_MAX_ATTEMPTS", os.Getenv("EXPERT_WORKER_MAX_ATTEMPTS"), 3, 20)
	if err != nil {
		return false, err
	}
	pollInterval, err := parseDuration("EXPERT_WORKER_POLL_INTERVAL", os.Getenv("EXPERT_WORKER_POLL_INTERVAL"), 500*time.Millisecond, 10*time.Millisecond, time.Minute)
	if err != nil {
		return false, err
	}
	leaseTTL, err := parseDuration("EXPERT_WORKER_LEASE_TTL", os.Getenv("EXPERT_WORKER_LEASE_TTL"), 2*time.Minute, 5*time.Second, 30*time.Minute)
	if err != nil {
		return false, err
	}
	renewInterval, err := parseDuration("EXPERT_WORKER_RENEW_INTERVAL", os.Getenv("EXPERT_WORKER_RENEW_INTERVAL"), leaseTTL/3, time.Second, leaseTTL-time.Millisecond)
	if err != nil {
		return false, err
	}
	retryBase, err := parseDuration("EXPERT_WORKER_RETRY_BASE_DELAY", os.Getenv("EXPERT_WORKER_RETRY_BASE_DELAY"), 2*time.Second, 10*time.Millisecond, 10*time.Minute)
	if err != nil {
		return false, err
	}
	retryMaximum, err := parseDuration("EXPERT_WORKER_RETRY_MAX_DELAY", os.Getenv("EXPERT_WORKER_RETRY_MAX_DELAY"), 2*time.Minute, retryBase, 24*time.Hour)
	if err != nil {
		return false, err
	}
	runner, err := worker.New(application, worker.OpenAIExecutor{
		Client:              client,
		Model:               model,
		ReasoningMode:       envOr("EXPERT_REASONING_MODE", "pro"),
		ReasoningEffort:     envOr("EXPERT_REASONING_EFFORT", "max"),
		MaximumOutputTokens: maximumOutputTokens,
	}, worker.Config{
		WorkerID:        envOr("EXPERT_WORKER_ID", defaultWorkerID()),
		Concurrency:     concurrency,
		MaximumAttempts: maximumAttempts,
		PollInterval:    pollInterval,
		LeaseTTL:        leaseTTL,
		RenewInterval:   renewInterval,
		RetryBaseDelay:  retryBase,
		RetryMaxDelay:   retryMaximum,
		Logger:          logger.With("component", "expert-worker"),
	})
	if err != nil {
		return false, err
	}
	go func() { done <- runner.Run(ctx) }()
	return true, nil
}

func pricingFromEnvironment() (*openaiapi.Pricing, error) {
	inputRaw := strings.TrimSpace(os.Getenv("EXPERT_INPUT_PER_MILLION_MINOR"))
	outputRaw := strings.TrimSpace(os.Getenv("EXPERT_OUTPUT_PER_MILLION_MINOR"))
	reasoningRaw := strings.TrimSpace(os.Getenv("EXPERT_REASONING_PER_MILLION_MINOR"))
	if inputRaw == "" && outputRaw == "" && reasoningRaw == "" {
		return nil, nil
	}
	if inputRaw == "" || outputRaw == "" {
		return nil, fmt.Errorf("%w: expert input and output prices are both required", core.ErrInvalidConfiguration)
	}
	input, err := parseNonNegativeInt64("EXPERT_INPUT_PER_MILLION_MINOR", inputRaw)
	if err != nil {
		return nil, err
	}
	output, err := parseNonNegativeInt64("EXPERT_OUTPUT_PER_MILLION_MINOR", outputRaw)
	if err != nil {
		return nil, err
	}
	reasoning := output
	if reasoningRaw != "" {
		reasoning, err = parseNonNegativeInt64("EXPERT_REASONING_PER_MILLION_MINOR", reasoningRaw)
		if err != nil {
			return nil, err
		}
	}
	return &openaiapi.Pricing{InputPerMillionMinor: input, OutputPerMillionMinor: output, ReasoningPerMillionMinor: reasoning}, nil
}

func parseNonNegativeInt64(name, raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%w: %s must be a non-negative integer", core.ErrInvalidConfiguration, name)
	}
	return value, nil
}

func parsePositiveInt(name, raw string, fallback, maximum int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 || value > maximum {
		return 0, fmt.Errorf("%w: %s must be between 1 and %d", core.ErrInvalidConfiguration, name, maximum)
	}
	return value, nil
}

func parseDuration(name, raw string, fallback, minimum, maximum time.Duration) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%w: %s must be between %s and %s", core.ErrInvalidConfiguration, name, minimum, maximum)
	}
	return value, nil
}

func defaultWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "unknown-host"
	}
	return fmt.Sprintf("%s-%d-%s", hostname, os.Getpid(), idgen.New("worker"))
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	out := make([]string, 0)
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
