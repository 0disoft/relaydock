package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/auth/virtualkey"
	"github.com/your-org/ai-runtime-gateway/internal/persistence/postgres"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "issue":
		err = issue(os.Args[2:])
	case "revoke":
		err = revoke(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func issue(args []string) error {
	flags := flag.NewFlagSet("issue", flag.ContinueOnError)
	projectID := flags.String("project", "", "control.projects UUID")
	tenantID := flags.String("tenant", "", "optional owning organization UUID")
	scopesCSV := flags.String("scopes", "models:invoke", "comma-separated scopes")
	modelsCSV := flags.String("models", "", "comma-separated allowed virtual models; empty allows all")
	expires := flags.Duration("expires", 30*24*time.Hour, "key lifetime; zero creates a non-expiring key")
	if err := flags.Parse(args); err != nil {
		return err
	}
	authenticator, closeDatabase, err := openAuthenticator()
	if err != nil {
		return err
	}
	defer closeDatabase()
	var expiresAt time.Time
	if *expires < 0 {
		return fmt.Errorf("--expires cannot be negative")
	}
	if *expires > 0 {
		expiresAt = time.Now().UTC().Add(*expires)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	raw, record, err := authenticator.Issue(ctx, *tenantID, *projectID, splitCSV(*scopesCSV), splitCSV(*modelsCSV), expiresAt)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"apiKey":        raw,
		"virtualKeyId":  record.VirtualKeyID,
		"publicId":      record.PublicID,
		"tenantId":      record.TenantID,
		"projectId":     record.ProjectID,
		"scopes":        record.Scopes,
		"allowedModels": record.AllowedModels,
		"expiresAt":     record.ExpiresAt,
	})
}

func revoke(args []string) error {
	flags := flag.NewFlagSet("revoke", flag.ContinueOnError)
	keyID := flags.String("key-id", "", "control.virtual_keys UUID")
	projectID := flags.String("project", "", "control.projects UUID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	authenticator, closeDatabase, err := openAuthenticator()
	if err != nil {
		return err
	}
	defer closeDatabase()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := authenticator.Revoke(ctx, *keyID, *projectID); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"revoked": true, "virtualKeyId": *keyID})
}

func openAuthenticator() (*virtualkey.PostgresAuthenticator, func(), error) {
	databaseURL := firstNonEmpty(os.Getenv("GATEWAY_POSTGRES_URL"), os.Getenv("ARG_POSTGRES_URL"))
	if databaseURL == "" {
		return nil, nil, fmt.Errorf("GATEWAY_POSTGRES_URL or ARG_POSTGRES_URL is required")
	}
	pepper, err := decodePepper(os.Getenv("GATEWAY_VIRTUAL_KEY_PEPPER"), os.Getenv("GATEWAY_VIRTUAL_KEY_PEPPER_B64"))
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	database, err := postgres.OpenSQL(ctx, databaseURL)
	if err != nil {
		return nil, nil, err
	}
	authenticator, err := virtualkey.NewPostgresAuthenticator(database, pepper, envOr("GATEWAY_KEY_ENVIRONMENT", "live"))
	if err != nil {
		_ = database.Close()
		return nil, nil, err
	}
	return authenticator, func() { _ = database.Close() }, nil
}

func decodePepper(raw, encoded string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	encoded = strings.TrimSpace(encoded)
	if encoded != "" {
		decoded, err := base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(encoded)
		}
		if err != nil {
			return nil, fmt.Errorf("decode GATEWAY_VIRTUAL_KEY_PEPPER_B64: %w", err)
		}
		return decoded, nil
	}
	if raw == "" {
		return nil, fmt.Errorf("GATEWAY_VIRTUAL_KEY_PEPPER or GATEWAY_VIRTUAL_KEY_PEPPER_B64 is required")
	}
	return []byte(raw), nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
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

func usage() {
	_, _ = fmt.Fprintln(os.Stderr, "usage: keyctl issue|revoke [flags]")
}
