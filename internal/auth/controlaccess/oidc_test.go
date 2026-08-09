package controlaccess

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

const (
	testTenantID  = "00000000-0000-0000-0000-000000000001"
	testProjectID = "00000000-0000-0000-0000-000000000002"
)

func TestOIDCAuthenticatorVerifiesDiscoveryClaimsAndJWKSRotation(t *testing.T) {
	var state struct {
		sync.RWMutex
		key *rsa.PrivateKey
		kid string
	}
	state.key = generateRSAKey(t)
	state.kid = "key-1"
	var issuer string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeJSON(t, w, map[string]any{
				"issuer": issuer, "jwks_uri": issuer + "/keys",
				"authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token",
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/keys":
			state.RLock()
			defer state.RUnlock()
			writeJSON(t, w, map[string]any{"keys": []any{rsaJWK(&state.key.PublicKey, state.kid)}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL

	authenticator, err := NewOIDCAuthenticator(context.Background(), OIDCConfig{
		Issuer: issuer, Audience: "relaydock-control",
		RoleMappings: map[string]Role{"relay.viewer": RoleViewer},
		AllowPrivate: true, HTTPTimeout: 5 * time.Second,
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	claims := map[string]any{
		defaultOIDCRoleClaim: "relay.viewer", defaultOIDCTenantClaim: testTenantID,
		defaultOIDCProjectClaim: testProjectID,
	}
	first := signIDToken(t, state.key, state.kid, issuer, "relaydock-control", "human-123", time.Now().Add(time.Hour), claims)
	principal, ok := authenticator.AuthenticateBearer(context.Background(), "Bearer "+first)
	if !ok || principal.Role != RoleViewer || principal.TenantID != testTenantID || principal.ProjectID != testProjectID {
		t.Fatalf("unexpected OIDC principal: %#v, %v", principal, ok)
	}
	if principal.Subject == "human-123" || !strings.HasPrefix(principal.Subject, "oidc:") {
		t.Fatalf("OIDC subject was not pseudonymized: %q", principal.Subject)
	}

	state.Lock()
	state.key = generateRSAKey(t)
	state.kid = "key-2"
	rotatedKey := state.key
	rotatedKID := state.kid
	state.Unlock()
	rotated := signIDToken(t, rotatedKey, rotatedKID, issuer, "relaydock-control", "human-123", time.Now().Add(time.Hour), claims)
	if _, ok := authenticator.AuthenticateBearer(context.Background(), "Bearer "+rotated); !ok {
		t.Fatal("rejected token after JWKS key rotation")
	}
}

func TestOIDCAuthenticatorDeniesInvalidTokensAndPrivilegeClaims(t *testing.T) {
	key := generateRSAKey(t)
	issuer := "https://issuer.example.com"
	audience := "relaydock-control"
	config, err := normalizeOIDCConfig(OIDCConfig{
		Issuer: issuer, Audience: audience,
		RoleMappings: map[string]Role{"relay.viewer": RoleViewer, "relay.admin": RoleAdmin},
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier := oidc.NewVerifier(issuer, &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &oidc.Config{ClientID: audience})
	authenticator := newOIDCAuthenticator(config, verifier)
	validClaims := map[string]any{defaultOIDCRoleClaim: "relay.viewer", defaultOIDCTenantClaim: testTenantID}

	tests := []struct {
		name     string
		issuer   string
		audience string
		expires  time.Time
		claims   map[string]any
	}{
		{name: "wrong issuer", issuer: "https://attacker.example.com", audience: audience, expires: time.Now().Add(time.Hour), claims: validClaims},
		{name: "wrong audience", issuer: issuer, audience: "other-api", expires: time.Now().Add(time.Hour), claims: validClaims},
		{name: "wrong authorized party", issuer: issuer, audience: audience, expires: time.Now().Add(time.Hour), claims: map[string]any{defaultOIDCRoleClaim: "relay.viewer", "azp": "other-client"}},
		{name: "multiple audiences without authorized party", issuer: issuer, audience: audience, expires: time.Now().Add(time.Hour), claims: map[string]any{defaultOIDCRoleClaim: "relay.viewer", "aud": []string{audience, "other-api"}}},
		{name: "expired", issuer: issuer, audience: audience, expires: time.Now().Add(-time.Hour), claims: validClaims},
		{name: "not active", issuer: issuer, audience: audience, expires: time.Now().Add(time.Hour), claims: map[string]any{defaultOIDCRoleClaim: "relay.viewer", "nbf": time.Now().Add(time.Hour).Unix()}},
		{name: "unmapped role", issuer: issuer, audience: audience, expires: time.Now().Add(time.Hour), claims: map[string]any{defaultOIDCRoleClaim: "provider-owner"}},
		{name: "provider scope cannot become role", issuer: issuer, audience: audience, expires: time.Now().Add(time.Hour), claims: map[string]any{defaultOIDCRoleClaim: "admin"}},
		{name: "scoped admin", issuer: issuer, audience: audience, expires: time.Now().Add(time.Hour), claims: map[string]any{defaultOIDCRoleClaim: "relay.admin", defaultOIDCTenantClaim: testTenantID}},
		{name: "invalid tenant", issuer: issuer, audience: audience, expires: time.Now().Add(time.Hour), claims: map[string]any{defaultOIDCRoleClaim: "relay.viewer", defaultOIDCTenantClaim: "tenant-a"}},
		{name: "role array", issuer: issuer, audience: audience, expires: time.Now().Add(time.Hour), claims: map[string]any{defaultOIDCRoleClaim: []string{"relay.viewer"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token := signIDToken(t, key, "key-1", test.issuer, test.audience, "subject", test.expires, test.claims)
			if principal, ok := authenticator.AuthenticateBearer(context.Background(), "Bearer "+token); ok {
				t.Fatalf("accepted invalid token as %#v", principal)
			}
		})
	}
	multipleAudience := signIDToken(t, key, "key-1", issuer, audience, "subject", time.Now().Add(time.Hour), map[string]any{
		defaultOIDCRoleClaim: "relay.viewer", "aud": []string{audience, "other-api"}, "azp": audience,
	})
	if _, ok := authenticator.AuthenticateBearer(context.Background(), "Bearer "+multipleAudience); !ok {
		t.Fatal("rejected valid multiple-audience token with matching authorized party")
	}
	if _, ok := authenticator.AuthenticateBearer(context.Background(), "Bearer "+strings.Repeat("x", maximumBearerTokenBytes+1)); ok {
		t.Fatal("accepted oversized bearer token")
	}
}

func TestOIDCRoleMappingsAreStrict(t *testing.T) {
	if _, err := ParseOIDCRoleMappings(`{"team.viewer":"viewer","team.admin":"admin"}`); err != nil {
		t.Fatal(err)
	}
	invalid := []string{"{}", `{"team":"owner"}`, `{" team":"viewer","team":"viewer"}`, `{"team":"viewer"} trailing`}
	for _, raw := range invalid {
		if _, err := ParseOIDCRoleMappings(raw); err == nil {
			t.Fatalf("accepted invalid role mappings %q", raw)
		}
	}
}

func TestCombinedAuthenticatorPreservesStaticBootstrap(t *testing.T) {
	staticToken := strings.Repeat("s", 32)
	static, err := NewStaticAuthenticator([]CredentialConfig{CredentialForToken(staticToken, Principal{Subject: "gateway", Role: RoleGateway})})
	if err != nil {
		t.Fatal(err)
	}
	authenticator := NewAuthenticator(static, nil)
	principal, ok := authenticator.AuthenticateBearer(context.Background(), "bearer "+staticToken)
	if !ok || principal.Role != RoleGateway {
		t.Fatalf("static bootstrap failed through combined authenticator: %#v, %v", principal, ok)
	}
}

func generateRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func signIDToken(t *testing.T, key *rsa.PrivateKey, kid, issuer, audience, subject string, expires time.Time, extra map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "RS256", "kid": kid, "typ": "JWT"}
	claims := map[string]any{
		"iss": issuer, "aud": audience, "sub": subject,
		"iat": time.Now().Add(-time.Minute).Unix(), "exp": expires.Unix(),
	}
	for name, value := range extra {
		claims[name] = value
	}
	encodedHeader := encodeJSON(t, header)
	encodedClaims := encodeJSON(t, claims)
	signingInput := encodedHeader + "." + encodedClaims
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func encodeJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func rsaJWK(key *rsa.PublicKey, kid string) map[string]any {
	exponent := big.NewInt(int64(key.E)).Bytes()
	return map[string]any{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": kid,
		"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(exponent),
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Error(fmt.Errorf("encode test response: %w", err))
	}
}
