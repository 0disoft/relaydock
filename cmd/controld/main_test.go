package main

import (
	"strings"
	"testing"

	"github.com/0disoft/relaydock/internal/auth/controlaccess"
)

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

	authenticator, count, err := loadControlAuthenticator(legacyToken)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || !authenticator.Configured() {
		t.Fatalf("credential count=%d configured=%v, want 2 true", count, authenticator.Configured())
	}
	viewer, ok := authenticator.AuthenticateBearer("Bearer " + scopedToken)
	if !ok || viewer.Role != controlaccess.RoleViewer || viewer.ProjectID == "" {
		t.Fatalf("unexpected scoped principal: %#v, %v", viewer, ok)
	}
	legacy, ok := authenticator.AuthenticateBearer("Bearer " + legacyToken)
	if !ok || legacy.Role != controlaccess.RoleAdmin || !legacy.IsClusterScoped() {
		t.Fatalf("unexpected legacy principal: %#v, %v", legacy, ok)
	}
}

func TestLoadControlAuthenticatorRejectsInvalidStaticPolicy(t *testing.T) {
	t.Setenv("CONTROL_ACCESS_TOKENS_JSON", `[{"tokenSha256":"invalid","subject":"admin","role":"admin"}]`)
	if _, _, err := loadControlAuthenticator(""); err == nil {
		t.Fatal("accepted invalid token digest")
	}
}
