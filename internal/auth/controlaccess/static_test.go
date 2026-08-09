package controlaccess

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStaticAuthenticatorSetsPrincipalWithoutStoringRawToken(t *testing.T) {
	t.Parallel()
	token := strings.Repeat("a", 32)
	config := CredentialForToken(token, Principal{Subject: "gateway-1", Role: RoleGateway})
	authenticator, err := NewStaticAuthenticator([]CredentialConfig{config})
	if err != nil {
		t.Fatal(err)
	}
	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok || principal.Subject != "gateway-1" {
			t.Fatalf("unexpected principal: %#v, %v", principal, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/v1/snapshot", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status %d, want %d: %s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if strings.Contains(config.TokenSHA256, token) {
		t.Fatal("credential configuration retained the raw token")
	}
}

func TestStaticAuthenticatorRejectsMissingAndUnknownTokens(t *testing.T) {
	t.Parallel()
	config := CredentialForToken(strings.Repeat("a", 32), Principal{Subject: "admin-1", Role: RoleAdmin})
	authenticator, err := NewStaticAuthenticator([]CredentialConfig{config})
	if err != nil {
		t.Fatal(err)
	}
	handler := authenticator.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unauthenticated request reached protected handler")
	}))
	for _, header := range []string{"", "Bearer " + strings.Repeat("b", 32)} {
		request := httptest.NewRequest(http.MethodGet, "/v1/snapshot", nil)
		request.Header.Set("Authorization", header)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("header %q returned %d, want %d", header, response.Code, http.StatusUnauthorized)
		}
	}
}

func TestStaticAuthenticatorAuditsAuthenticationFailureWithoutToken(t *testing.T) {
	t.Parallel()
	config := CredentialForToken(strings.Repeat("a", 32), Principal{Subject: "admin-1", Role: RoleAdmin})
	authenticator, err := NewStaticAuthenticator([]CredentialConfig{config})
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan AuditEvent, 1)
	handler := authenticator.MiddlewareWithAudit(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unauthenticated request reached protected handler")
	}), func(_ context.Context, event AuditEvent) { events <- event })
	request := httptest.NewRequest(http.MethodGet, "/v1/snapshot", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("b", 32))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	event := <-events
	if event.Action != ActionAuthenticate || event.Allowed || event.Subject != "" || event.Reason != "authentication_required" {
		t.Fatalf("unexpected authentication audit event: %#v", event)
	}
}

func TestParseCredentialConfigRejectsUnknownTrailingAndDuplicateValues(t *testing.T) {
	t.Parallel()
	digest := CredentialForToken(strings.Repeat("a", 32), Principal{Subject: "admin-1", Role: RoleAdmin}).TokenSHA256
	for _, raw := range []string{
		`[{"tokenSha256":"` + digest + `","subject":"admin-1","role":"admin","unexpected":true}]`,
		`[{"tokenSha256":"` + digest + `","subject":"admin-1","role":"admin"}] {}`,
	} {
		if _, err := ParseCredentialConfig(raw); err == nil {
			t.Fatalf("accepted invalid config %s", raw)
		}
	}
	configs := []CredentialConfig{
		{TokenSHA256: digest, Subject: "admin-1", Role: RoleAdmin},
		{TokenSHA256: digest, Subject: "admin-2", Role: RoleAdmin},
	}
	if _, err := NewStaticAuthenticator(configs); err == nil {
		t.Fatal("accepted duplicate token digest")
	}
}
