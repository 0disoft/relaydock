package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/0disoft/relaydock/internal/buildinfo"
	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/idgen"
	"github.com/0disoft/relaydock/internal/observability"
	"github.com/0disoft/relaydock/internal/outbox"
	"github.com/0disoft/relaydock/internal/persistence/postgres"
	"github.com/0disoft/relaydock/internal/serverutil"
	"github.com/0disoft/relaydock/internal/transport/apiutil"
)

func main() {
	logger := observability.NewLogger(os.Stdout, slog.LevelInfo).With("service", "outboxd")
	configuration, err := loadConfiguration()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 15*time.Second)
	database, err := postgres.OpenSQL(startupCtx, configuration.DatabaseURL)
	cancelStartup()
	if err != nil {
		logger.Error("open PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	repository, err := postgres.NewOutboxRepository(database)
	if err != nil {
		logger.Error("configure outbox repository", "error", err)
		os.Exit(1)
	}
	publisher, err := outbox.NewWebhookPublisher(outbox.WebhookConfig{
		Endpoint:          configuration.WebhookURL,
		Secret:            configuration.WebhookSecret,
		UserAgent:         "ai-runtime-gateway-outbox/" + buildinfo.Version,
		AllowInsecureHTTP: configuration.AllowInsecureHTTP,
		Headers:           configuration.WebhookHeaders,
	})
	if err != nil {
		logger.Error("configure outbox webhook", "error", err)
		os.Exit(1)
	}
	worker, err := outbox.NewWorker(repository, publisher, outbox.WorkerConfig{
		WorkerID:          configuration.WorkerID,
		BatchSize:         configuration.BatchSize,
		Concurrency:       configuration.Concurrency,
		LeaseTTL:          configuration.LeaseTTL,
		PollInterval:      configuration.PollInterval,
		PublishTimeout:    configuration.PublishTimeout,
		RepositoryTimeout: configuration.RepositoryTimeout,
		MaximumAttempts:   configuration.MaximumAttempts,
		RetryBase:         configuration.RetryBase,
		RetryMaximum:      configuration.RetryMaximum,
		JitterRatio:       configuration.JitterRatio,
		Logger:            logger,
	})
	if err != nil {
		logger.Error("configure outbox worker", "error", err)
		os.Exit(1)
	}

	if err := serverutil.RequireAuthenticationOutsideLoopback(configuration.HealthAddress, configuration.BearerToken); err != nil {
		logger.Error("unsafe health endpoint configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	type componentResult struct {
		name string
		err  error
	}
	done := make(chan componentResult, 2)
	go func() { done <- componentResult{name: "worker", err: worker.Run(ctx)} }()
	go func() {
		handler := apiutil.RequireBearer(configuration.BearerToken, healthHandler(repository, configuration), "/healthz", "/readyz")
		done <- componentResult{name: "health-server", err: serverutil.RunContext(ctx, configuration.HealthAddress, handler, logger)}
	}()

	logger.Info(
		"starting transactional outbox delivery",
		"version", buildinfo.Version,
		"workerId", configuration.WorkerID,
		"batchSize", configuration.BatchSize,
		"concurrency", configuration.Concurrency,
		"healthAddress", configuration.HealthAddress,
		"authentication", configuration.BearerToken != "",
	)

	var exitErr error
	completed := 0
	select {
	case result := <-done:
		completed++
		if result.err != nil && !errors.Is(result.err, context.Canceled) {
			exitErr = fmt.Errorf("outbox %s stopped: %w", result.name, result.err)
		} else if ctx.Err() == nil {
			exitErr = fmt.Errorf("outbox %s stopped unexpectedly", result.name)
		}
	case <-ctx.Done():
	}
	stop()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for completed < 2 {
		select {
		case <-done:
			completed++
		case <-deadline.C:
			logger.Error("outbox shutdown timed out", "completedComponents", completed)
			completed = 2
		}
	}
	if exitErr != nil {
		logger.Error("outbox service stopped", "error", exitErr)
		os.Exit(1)
	}
}

type configuration struct {
	DatabaseURL       string
	WebhookURL        string
	WebhookSecret     []byte
	WebhookHeaders    map[string]string
	AllowInsecureHTTP bool
	WorkerID          string
	BatchSize         int
	Concurrency       int
	LeaseTTL          time.Duration
	PollInterval      time.Duration
	PublishTimeout    time.Duration
	RepositoryTimeout time.Duration
	MaximumAttempts   int
	RetryBase         time.Duration
	RetryMaximum      time.Duration
	JitterRatio       float64
	HealthAddress     string
	BearerToken       string
}

func loadConfiguration() (configuration, error) {
	value := configuration{
		DatabaseURL:   firstNonEmpty(os.Getenv("OUTBOX_POSTGRES_URL"), os.Getenv("ARG_POSTGRES_URL"), os.Getenv("DATABASE_URL")),
		WebhookURL:    strings.TrimSpace(os.Getenv("OUTBOX_WEBHOOK_URL")),
		WorkerID:      strings.TrimSpace(os.Getenv("OUTBOX_WORKER_ID")),
		HealthAddress: envOr("OUTBOX_HEALTH_ADDRESS", "127.0.0.1:8083"),
		BearerToken:   strings.TrimSpace(os.Getenv("OUTBOX_BEARER_TOKEN")),
	}
	if value.DatabaseURL == "" || value.WebhookURL == "" {
		return configuration{}, fmt.Errorf("%w: OUTBOX_POSTGRES_URL/ARG_POSTGRES_URL and OUTBOX_WEBHOOK_URL are required", core.ErrInvalidConfiguration)
	}
	if value.WorkerID == "" {
		hostname, _ := os.Hostname()
		value.WorkerID = strings.Trim(strings.TrimSpace(hostname)+"-"+idgen.New("outbox"), "-")
	}
	var err error
	if value.WebhookSecret, err = decodeSecret("OUTBOX_WEBHOOK_SECRET", "OUTBOX_WEBHOOK_SECRET_B64"); err != nil {
		return configuration{}, err
	}
	if value.WebhookHeaders, err = parseHeaders(os.Getenv("OUTBOX_WEBHOOK_HEADERS_JSON")); err != nil {
		return configuration{}, err
	}
	if value.AllowInsecureHTTP, err = parseBool("OUTBOX_ALLOW_INSECURE_HTTP", false); err != nil {
		return configuration{}, err
	}
	if value.BatchSize, err = parseInt("OUTBOX_BATCH_SIZE", 32, 1, 1_000); err != nil {
		return configuration{}, err
	}
	if value.Concurrency, err = parseInt("OUTBOX_CONCURRENCY", 4, 1, 256); err != nil {
		return configuration{}, err
	}
	if value.MaximumAttempts, err = parseInt("OUTBOX_MAX_ATTEMPTS", 12, 1, 1_000); err != nil {
		return configuration{}, err
	}
	if value.LeaseTTL, err = parseDuration("OUTBOX_LEASE_TTL", time.Minute, time.Second, time.Hour); err != nil {
		return configuration{}, err
	}
	if value.PollInterval, err = parseDuration("OUTBOX_POLL_INTERVAL", time.Second, 10*time.Millisecond, time.Minute); err != nil {
		return configuration{}, err
	}
	if value.PublishTimeout, err = parseDuration("OUTBOX_PUBLISH_TIMEOUT", 30*time.Second, 100*time.Millisecond, 30*time.Minute); err != nil {
		return configuration{}, err
	}
	if value.RepositoryTimeout, err = parseDuration("OUTBOX_REPOSITORY_TIMEOUT", 10*time.Second, 100*time.Millisecond, 5*time.Minute); err != nil {
		return configuration{}, err
	}
	if value.RetryBase, err = parseDuration("OUTBOX_RETRY_BASE_DELAY", time.Second, 10*time.Millisecond, time.Hour); err != nil {
		return configuration{}, err
	}
	if value.RetryMaximum, err = parseDuration("OUTBOX_RETRY_MAX_DELAY", 15*time.Minute, value.RetryBase, 7*24*time.Hour); err != nil {
		return configuration{}, err
	}
	if value.JitterRatio, err = parseFloat("OUTBOX_RETRY_JITTER_RATIO", 0.2, 0, 1); err != nil {
		return configuration{}, err
	}
	return value, nil
}

func healthHandler(repository outbox.AdminRepository, configuration configuration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		apiutil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": buildinfo.Version})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), configuration.RepositoryTimeout)
		defer cancel()
		if _, err := repository.Status(ctx, time.Now().UTC()); err != nil {
			apiutil.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "reason": "outbox_storage_unavailable"})
			return
		}
		apiutil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ready"})
	})
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), configuration.RepositoryTimeout)
		defer cancel()
		status, err := repository.Status(ctx, time.Now().UTC())
		if err != nil {
			apiutil.WriteError(w, http.StatusServiceUnavailable, "outbox_status_unavailable", err.Error())
			return
		}
		apiutil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":   status,
			"workerId": configuration.WorkerID,
			"version":  buildinfo.Version,
		})
	})
	return mux
}

func decodeSecret(rawName, encodedName string) ([]byte, error) {
	raw := os.Getenv(rawName)
	encoded := strings.TrimSpace(os.Getenv(encodedName))
	if raw == "" && encoded == "" {
		return nil, fmt.Errorf("%w: %s or %s is required", core.ErrInvalidConfiguration, rawName, encodedName)
	}
	if encoded == "" {
		return []byte(raw), nil
	}
	var lastErr error
	for _, encoding := range []*base64.Encoding{base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding} {
		decoded, err := encoding.DecodeString(encoded)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("%w: decode %s: %v", core.ErrInvalidConfiguration, encodedName, lastErr)
}

func parseHeaders(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		return nil, fmt.Errorf("%w: OUTBOX_WEBHOOK_HEADERS_JSON: %v", core.ErrInvalidConfiguration, err)
	}
	return headers, nil
}

func parseDuration(name string, fallback, minimum, maximum time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%w: %s must be between %s and %s", core.ErrInvalidConfiguration, name, minimum, maximum)
	}
	return value, nil
}

func parseInt(name string, fallback, minimum, maximum int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%w: %s must be between %d and %d", core.ErrInvalidConfiguration, name, minimum, maximum)
	}
	return value, nil
}

func parseFloat(name string, fallback, minimum, maximum float64) (float64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%w: %s must be between %.2f and %.2f", core.ErrInvalidConfiguration, name, minimum, maximum)
	}
	return value, nil
}

func parseBool(name string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%w: %s must be a boolean", core.ErrInvalidConfiguration, name)
	}
	return value, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
