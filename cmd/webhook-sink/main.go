package main

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/0disoft/relaydock/internal/buildinfo"
	"github.com/0disoft/relaydock/internal/observability"
	"github.com/0disoft/relaydock/internal/outbox/webhooksink"
	"github.com/0disoft/relaydock/internal/serverutil"
	"github.com/0disoft/relaydock/internal/transport/apiutil"
)

func main() {
	logger := observability.NewLogger(os.Stdout, slog.LevelInfo).With("service", "webhook-sink")
	address := envOr("WEBHOOK_SINK_ADDRESS", "127.0.0.1:8090")
	bearer := strings.TrimSpace(os.Getenv("WEBHOOK_SINK_BEARER_TOKEN"))
	if err := serverutil.RequireAuthenticationOutsideLoopback(address, bearer); err != nil {
		logger.Error("unsafe webhook sink configuration", "error", err)
		os.Exit(1)
	}
	secret, err := decodeSecret()
	if err != nil {
		logger.Error("load webhook sink secret", "error", err)
		os.Exit(1)
	}
	maximumBody, err := strconv.ParseInt(envOr("WEBHOOK_SINK_MAX_BODY_BYTES", "2097152"), 10, 64)
	if err != nil || maximumBody <= 0 {
		logger.Error("invalid WEBHOOK_SINK_MAX_BODY_BYTES")
		os.Exit(1)
	}
	handler, err := webhooksink.New(webhooksink.Config{Secret: secret, MaximumBodyBytes: maximumBody, Logger: logger})
	if err != nil {
		logger.Error("configure webhook sink", "error", err)
		os.Exit(1)
	}
	secured := apiutil.RequireBearer(bearer, handler.HTTPHandler(), "/healthz", "/events")
	logger.Info("starting reference webhook sink", "address", address, "version", buildinfo.Version)
	if err := serverutil.Run(address, secured, logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func decodeSecret() ([]byte, error) {
	if encoded := strings.TrimSpace(os.Getenv("WEBHOOK_SINK_SECRET_B64")); encoded != "" {
		for _, encoding := range []*base64.Encoding{base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding} {
			if decoded, err := encoding.DecodeString(encoded); err == nil {
				return decoded, nil
			}
		}
		return nil, fmt.Errorf("decode WEBHOOK_SINK_SECRET_B64")
	}
	if raw := os.Getenv("WEBHOOK_SINK_SECRET"); raw != "" {
		return []byte(raw), nil
	}
	return nil, fmt.Errorf("WEBHOOK_SINK_SECRET or WEBHOOK_SINK_SECRET_B64 is required")
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
