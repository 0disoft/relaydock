package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/auth/controlaccess"
	"github.com/0disoft/relaydock/internal/control/snapshot"
	"github.com/0disoft/relaydock/internal/observability"
	persistencepostgres "github.com/0disoft/relaydock/internal/persistence/postgres"
	"github.com/0disoft/relaydock/internal/security/serversecrets"
	"github.com/0disoft/relaydock/internal/serverutil"
	"github.com/0disoft/relaydock/internal/transport/controlhttp"
)

func main() {
	logger := observability.NewLogger(os.Stdout, slog.LevelInfo).With("service", "controld")
	address := envOr("CONTROL_ADDRESS", "127.0.0.1:8081")
	token := strings.TrimSpace(os.Getenv("CONTROL_BEARER_TOKEN"))
	authenticator, credentialCount, oidcConfigured, err := loadControlAuthenticator(context.Background(), token)
	if err != nil {
		logger.Error("load control access policy", "error", err)
		os.Exit(1)
	}
	authenticationMarker := ""
	if authenticator.Configured() {
		authenticationMarker = "configured"
	}
	if err := serverutil.RequireAuthenticationOutsideLoopback(address, authenticationMarker); err != nil {
		logger.Error("unsafe control-plane configuration", "error", err)
		os.Exit(1)
	}

	store, storeDescription, closeStore, err := openSnapshotStore()
	if err != nil {
		logger.Error("open control snapshot store", "error", err)
		os.Exit(1)
	}
	defer closeStore()

	secretResolver, err := serversecrets.NewDefaultResolver()
	if err != nil {
		logger.Error("configure server-secret resolver", "error", err)
		os.Exit(1)
	}
	signingContext, cancelSigning := context.WithTimeout(context.Background(), 15*time.Second)
	signer, signerDescription, err := loadSigner(signingContext, secretResolver)
	cancelSigning()
	if err != nil {
		logger.Error("load control signing key", "error", err)
		os.Exit(1)
	}
	verifier, trustedKeyIDs, err := loadVerifier(signer)
	if err != nil {
		logger.Error("load control verification key ring", "error", err)
		os.Exit(1)
	}
	api, err := controlhttp.NewAPIWithVerifier(store, signer, verifier, signer.PublicKey)
	if err != nil {
		logger.Error("initialise control API", "error", err)
		os.Exit(1)
	}
	accessAudit := func(ctx context.Context, event controlaccess.AuditEvent) {
		logger.InfoContext(ctx, "control access decision",
			"subject", event.Subject,
			"role", event.Role,
			"tenantId", event.TenantID,
			"projectId", event.ProjectID,
			"action", event.Action,
			"allowed", event.Allowed,
			"reason", event.Reason,
			"policyVersion", event.PolicyVersion,
		)
	}
	api.SetAccessAudit(accessAudit)
	var handler http.Handler
	if authenticator.Configured() {
		handler = authenticator.MiddlewareWithAudit(api.Handler(), accessAudit, "/healthz", "/readyz")
	} else {
		developmentPrincipal := controlaccess.Principal{Subject: "loopback-development", Role: controlaccess.RoleAdmin}
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			api.Handler().ServeHTTP(w, r.WithContext(controlaccess.WithPrincipal(r.Context(), developmentPrincipal)))
		})
	}
	logger.Info(
		"starting control plane",
		"address", address,
		"authentication", authenticator.Configured(),
		"accessCredentials", credentialCount,
		"oidc", oidcConfigured,
		"snapshotStore", storeDescription,
		"signingKey", signerDescription,
		"signingKeyId", signer.SigningKeyID(),
		"trustedSigningKeyIds", trustedKeyIDs,
	)
	if err := serverutil.Run(address, handler, logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func loadControlAuthenticator(ctx context.Context, legacyToken string) (*controlaccess.Authenticator, int, bool, error) {
	configs, err := controlaccess.ParseCredentialConfig(os.Getenv("CONTROL_ACCESS_TOKENS_JSON"))
	if err != nil {
		return nil, 0, false, err
	}
	if legacyToken = strings.TrimSpace(legacyToken); legacyToken != "" {
		configs = append(configs, controlaccess.CredentialForToken(legacyToken, controlaccess.Principal{
			Subject: "legacy-control-operator", Role: controlaccess.RoleAdmin,
		}))
	}
	authenticator, err := controlaccess.NewStaticAuthenticator(configs)
	if err != nil {
		return nil, 0, false, err
	}
	oidcAuthenticator, err := loadOIDCAuthenticator(ctx)
	if err != nil {
		return nil, 0, false, err
	}
	return controlaccess.NewAuthenticator(authenticator, oidcAuthenticator), len(configs), oidcAuthenticator != nil, nil
}

func loadOIDCAuthenticator(ctx context.Context) (*controlaccess.OIDCAuthenticator, error) {
	issuer := strings.TrimSpace(os.Getenv("CONTROL_OIDC_ISSUER"))
	audience := strings.TrimSpace(os.Getenv("CONTROL_OIDC_AUDIENCE"))
	rawMappings := strings.TrimSpace(os.Getenv("CONTROL_OIDC_ROLE_MAPPINGS_JSON"))
	if issuer == "" {
		if audience != "" || rawMappings != "" || firstNonEmpty(
			os.Getenv("CONTROL_OIDC_ROLE_CLAIM"), os.Getenv("CONTROL_OIDC_TENANT_CLAIM"),
			os.Getenv("CONTROL_OIDC_PROJECT_CLAIM"), os.Getenv("CONTROL_OIDC_ALLOW_PRIVATE_ISSUER"),
			os.Getenv("CONTROL_OIDC_HTTP_TIMEOUT"),
		) != "" {
			return nil, fmt.Errorf("CONTROL_OIDC_ISSUER is required when any OIDC setting is configured")
		}
		return nil, nil
	}
	mappings, err := controlaccess.ParseOIDCRoleMappings(rawMappings)
	if err != nil {
		return nil, err
	}
	allowPrivate, err := booleanEnv("CONTROL_OIDC_ALLOW_PRIVATE_ISSUER", false)
	if err != nil {
		return nil, err
	}
	timeout, err := durationEnv("CONTROL_OIDC_HTTP_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, err
	}
	return controlaccess.NewOIDCAuthenticator(ctx, controlaccess.OIDCConfig{
		Issuer: issuer, Audience: audience,
		RoleClaim:    strings.TrimSpace(os.Getenv("CONTROL_OIDC_ROLE_CLAIM")),
		TenantClaim:  strings.TrimSpace(os.Getenv("CONTROL_OIDC_TENANT_CLAIM")),
		ProjectClaim: strings.TrimSpace(os.Getenv("CONTROL_OIDC_PROJECT_CLAIM")),
		RoleMappings: mappings, AllowPrivate: allowPrivate, HTTPTimeout: timeout,
	}, nil)
}

func openSnapshotStore() (snapshot.Store, string, func(), error) {
	kind := strings.ToLower(envOr("CONTROL_SNAPSHOT_STORE", "local"))
	switch kind {
	case "local":
		statePath := envOr("CONTROL_STATE_PATH", filepath.Join("data", "control", "snapshot.json"))
		store, err := snapshot.OpenLocalStore(statePath)
		return store, "local:" + statePath, func() {}, err
	case "postgres":
		databaseURL := firstNonEmpty(
			os.Getenv("CONTROL_POSTGRES_URL"),
			os.Getenv("ARG_POSTGRES_URL"),
			os.Getenv("DATABASE_URL"),
		)
		if databaseURL == "" {
			return nil, "", func() {}, fmt.Errorf("CONTROL_SNAPSHOT_STORE=postgres requires CONTROL_POSTGRES_URL, ARG_POSTGRES_URL, or DATABASE_URL")
		}
		openTimeout, err := durationEnv("CONTROL_POSTGRES_OPEN_TIMEOUT", 15*time.Second)
		if err != nil {
			return nil, "", func() {}, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), openTimeout)
		database, err := persistencepostgres.OpenSQL(ctx, databaseURL)
		cancel()
		if err != nil {
			return nil, "", func() {}, err
		}
		pollInterval, err := durationEnv("CONTROL_SNAPSHOT_POLL_INTERVAL", 2*time.Second)
		if err != nil {
			_ = database.Close()
			return nil, "", func() {}, err
		}
		store, err := persistencepostgres.NewRuntimeSnapshotStore(database, persistencepostgres.RuntimeSnapshotStoreOptions{PollInterval: pollInterval})
		if err != nil {
			_ = database.Close()
			return nil, "", func() {}, err
		}
		return store, "postgres", func() { _ = database.Close() }, nil
	default:
		return nil, "", func() {}, fmt.Errorf("unsupported CONTROL_SNAPSHOT_STORE %q; expected local or postgres", kind)
	}
}

type signingSecretResolver interface {
	Resolve(context.Context, string) ([]byte, error)
}

func loadSigner(ctx context.Context, secretResolver signingSecretResolver) (snapshot.Ed25519Signer, string, error) {
	keyID := strings.TrimSpace(os.Getenv("CONTROL_SIGNING_KEY_ID"))
	if encoded := strings.TrimSpace(os.Getenv("CONTROL_SIGNING_PRIVATE_KEY")); encoded != "" {
		signer, err := snapshot.ParseSigningPrivateKey(encoded)
		if err == nil && keyID != "" {
			signer = snapshot.NewEd25519SignerWithKeyID(keyID, signer.PrivateKey, signer.PublicKey)
		}
		return signer, "environment-secret", err
	}
	if reference := os.Getenv("CONTROL_SIGNING_PRIVATE_KEY_REF"); strings.TrimSpace(reference) != "" {
		if secretResolver == nil {
			return snapshot.Ed25519Signer{}, "", fmt.Errorf("CONTROL_SIGNING_PRIVATE_KEY_REF requires a server-secret resolver")
		}
		raw, err := secretResolver.Resolve(ctx, reference)
		if err != nil {
			return snapshot.Ed25519Signer{}, "", fmt.Errorf("resolve control signing private-key reference: %w", err)
		}
		defer clear(raw)
		signer, err := snapshot.ParseSigningPrivateKeyBytes(raw)
		if err == nil && keyID != "" {
			signer = snapshot.NewEd25519SignerWithKeyID(keyID, signer.PrivateKey, signer.PublicKey)
		}
		return signer, "server-secret-reference", err
	}
	keyPath := envOr("CONTROL_SIGNING_KEY_PATH", filepath.Join("data", "control", "signing.key"))
	signer, err := snapshot.LoadOrCreateSigningKey(keyPath)
	if err == nil && keyID != "" {
		signer = snapshot.NewEd25519SignerWithKeyID(keyID, signer.PrivateKey, signer.PublicKey)
	}
	return signer, "local-file:" + keyPath, err
}

func loadVerifier(active snapshot.Ed25519Signer) (snapshot.Verifier, []string, error) {
	keys := []snapshot.TrustedPublicKey{{ID: active.SigningKeyID(), PublicKey: active.PublicKey}}
	additional, err := snapshot.ParseTrustedPublicKeys(os.Getenv("CONTROL_TRUSTED_SIGNING_PUBLIC_KEYS"))
	if err != nil {
		return nil, nil, err
	}
	for _, candidate := range additional {
		id := strings.TrimSpace(candidate.ID)
		if id == "" {
			id = snapshot.DeriveKeyID(candidate.PublicKey)
		}
		if id == active.SigningKeyID() {
			continue
		}
		keys = append(keys, candidate)
	}
	allowLegacy, err := booleanEnv("CONTROL_ALLOW_LEGACY_SIGNING_KEY_ID", true)
	if err != nil {
		return nil, nil, err
	}
	ring, err := snapshot.NewEd25519KeyRing(keys, allowLegacy)
	if err != nil {
		return nil, nil, err
	}
	return ring, ring.KeyIDs(), nil
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", name)
	}
	return parsed, nil
}

func booleanEnv(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean", name)
	}
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
