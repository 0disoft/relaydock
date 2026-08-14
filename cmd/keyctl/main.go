package main

/* llmnav/1 module
id=relaydock.command.keyctl
role=Issue and revoke project-scoped gateway virtual keys against the authoritative PostgreSQL control store.
owns=virtual key operator CLI|virtual key issuance output|virtual key revocation command
excludes=per-request key authentication|MCP token issuance
search=issue gateway key|revoke virtual key|project key CLI
invariant=The plaintext virtual-key secret is emitted only by the issue command and is never read back from storage.
stability=contract
*/

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/auth/virtualkey"
	"github.com/0disoft/relaydock/internal/persistence/postgres"
	"github.com/0disoft/relaydock/internal/security/serversecrets"
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
	setupContext, cancelSetup := context.WithTimeout(context.Background(), 15*time.Second)
	authenticator, closeDatabase, err := openAuthenticator(setupContext)
	cancelSetup()
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
	operationContext, cancelOperation := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelOperation()
	raw, record, err := authenticator.Issue(operationContext, *tenantID, *projectID, splitCSV(*scopesCSV), splitCSV(*modelsCSV), expiresAt)
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
	setupContext, cancelSetup := context.WithTimeout(context.Background(), 15*time.Second)
	authenticator, closeDatabase, err := openAuthenticator(setupContext)
	cancelSetup()
	if err != nil {
		return err
	}
	defer closeDatabase()
	operationContext, cancelOperation := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelOperation()
	if err := authenticator.Revoke(operationContext, *keyID, *projectID); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"revoked": true, "virtualKeyId": *keyID})
}

func openAuthenticator(ctx context.Context) (*virtualkey.PostgresAuthenticator, func(), error) {
	databaseURL := firstNonEmpty(os.Getenv("GATEWAY_POSTGRES_URL"), os.Getenv("ARG_POSTGRES_URL"))
	if databaseURL == "" {
		return nil, nil, fmt.Errorf("GATEWAY_POSTGRES_URL or ARG_POSTGRES_URL is required")
	}
	secretResolver, err := serversecrets.NewDefaultResolver()
	if err != nil {
		return nil, nil, err
	}
	pepper, err := virtualkey.ResolvePepper(
		ctx, secretResolver,
		os.Getenv("GATEWAY_VIRTUAL_KEY_PEPPER"),
		os.Getenv("GATEWAY_VIRTUAL_KEY_PEPPER_B64"),
		os.Getenv("GATEWAY_VIRTUAL_KEY_PEPPER_REF"),
	)
	if err != nil {
		return nil, nil, err
	}
	defer clear(pepper)
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
