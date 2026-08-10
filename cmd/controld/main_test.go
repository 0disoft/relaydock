package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/relaydock/internal/auth/controlaccess"
)

type controlSigningResolver struct {
	value []byte
	calls int
}

func (r *controlSigningResolver) Resolve(ctx context.Context, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.calls++
	return append([]byte(nil), r.value...), nil
}

func TestLoadControlAuthenticatorCombinesScopedAndLegacyCredentials(t *testing.T) {
	scopedToken := strings.Repeat("a", 32)
	legacyToken := strings.Repeat("b", 32)
	scoped := controlaccess.CredentialForToken(scopedToken, controlaccess.Principal{
		Subject:   "project-viewer",
		Role:      controlaccess.RoleViewer,
		TenantID:  "00000000-0000-0000-0000-000000000001",
		ProjectID: "00000000-0000-0000-0000-000000000002",
	})
	t.Setenv("CONTROL_ACCESS_TOKENS_JSON", `[{"tokenSha256":"`+scoped.TokenSHA256+`","subject":"project-viewer","role":"viewer","tenantId":"00000000-0000-0000-0000-000000000001","projectId":"00000000-0000-0000-0000-000000000002"}]`)

	authenticator, count, oidcConfigured, err := loadControlAuthenticator(context.Background(), legacyToken)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || oidcConfigured || !authenticator.Configured() {
		t.Fatalf("credential count=%d configured=%v, want 2 true", count, authenticator.Configured())
	}
	viewer, ok := authenticator.AuthenticateBearer(context.Background(), "Bearer "+scopedToken)
	if !ok || viewer.Role != controlaccess.RoleViewer || viewer.ProjectID == "" {
		t.Fatalf("unexpected scoped principal: %#v, %v", viewer, ok)
	}
	legacy, ok := authenticator.AuthenticateBearer(context.Background(), "Bearer "+legacyToken)
	if !ok || legacy.Role != controlaccess.RoleAdmin || !legacy.IsClusterScoped() {
		t.Fatalf("unexpected legacy principal: %#v, %v", legacy, ok)
	}
}

func TestLoadControlAuthenticatorRejectsInvalidStaticPolicy(t *testing.T) {
	t.Setenv("CONTROL_ACCESS_TOKENS_JSON", `[{"tokenSha256":"invalid","subject":"admin","role":"admin"}]`)
	if _, _, _, err := loadControlAuthenticator(context.Background(), ""); err == nil {
		t.Fatal("accepted invalid token digest")
	}
}

func TestLoadOIDCAuthenticatorRejectsPartialConfiguration(t *testing.T) {
	t.Setenv("CONTROL_OIDC_AUDIENCE", "relaydock-control")
	if _, err := loadOIDCAuthenticator(context.Background()); err == nil {
		t.Fatal("accepted OIDC audience without issuer")
	}
}

func TestLoadSignerUsesRawServerSecretAndKeyID(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	t.Setenv("CONTROL_SIGNING_PRIVATE_KEY", "")
	t.Setenv("CONTROL_SIGNING_PRIVATE_KEY_REF", "gcp-sm:projects/p/secrets/control-signing/versions/8")
	t.Setenv("CONTROL_SIGNING_KEY_ID", "2026-q3")
	resolver := &controlSigningResolver{value: seed}
	signer, description, err := loadSigner(context.Background(), resolver)
	if err != nil {
		t.Fatal(err)
	}
	if resolver.calls != 1 || description != "server-secret-reference" || signer.SigningKeyID() != "2026-q3" {
		t.Fatalf("calls=%d description=%q keyID=%q", resolver.calls, description, signer.SigningKeyID())
	}
	if string(signer.PrivateKey) != string(ed25519.NewKeyFromSeed(seed)) {
		t.Fatal("resolved seed did not produce the expected private key")
	}
}

func TestLoadSignerKeepsEnvironmentPrecedence(t *testing.T) {
	seed := []byte(strings.Repeat("e", ed25519.SeedSize))
	t.Setenv("CONTROL_SIGNING_PRIVATE_KEY", base64.RawURLEncoding.EncodeToString(seed))
	t.Setenv("CONTROL_SIGNING_PRIVATE_KEY_REF", "gcp-sm:projects/p/secrets/control-signing/versions/8")
	resolver := &controlSigningResolver{value: []byte(strings.Repeat("r", ed25519.SeedSize))}
	_, description, err := loadSigner(context.Background(), resolver)
	if err != nil {
		t.Fatal(err)
	}
	if resolver.calls != 0 || description != "environment-secret" {
		t.Fatalf("calls=%d description=%q", resolver.calls, description)
	}
}

func TestLoadSignerRejectsInvalidResolvedMaterialWithoutDisclosure(t *testing.T) {
	secretText := "do-not-expose-signing-secret"
	t.Setenv("CONTROL_SIGNING_PRIVATE_KEY", "")
	t.Setenv("CONTROL_SIGNING_PRIVATE_KEY_REF", "gcp-sm:projects/p/secrets/control-signing/versions/8")
	resolver := &controlSigningResolver{value: []byte(secretText)}
	_, _, err := loadSigner(context.Background(), resolver)
	if err == nil {
		t.Fatal("accepted invalid resolved signing key")
	}
	if strings.Contains(err.Error(), secretText) {
		t.Fatalf("error exposed signing key: %v", err)
	}
}

func TestLoadSignerReferenceFailureDoesNotCreateLocalKey(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "signing.key")
	t.Setenv("CONTROL_SIGNING_PRIVATE_KEY", "")
	t.Setenv("CONTROL_SIGNING_PRIVATE_KEY_REF", "gcp-sm:projects/p/secrets/control-signing/versions/8")
	t.Setenv("CONTROL_SIGNING_KEY_PATH", keyPath)
	if _, _, err := loadSigner(context.Background(), nil); err == nil {
		t.Fatal("accepted a signing-key reference without a resolver")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("local key path changed after reference failure: %v", err)
	}
}
