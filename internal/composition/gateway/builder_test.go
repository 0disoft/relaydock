package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/credentials"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/compiler"
	"github.com/0disoft/relaydock/internal/provider"
)

func TestBuildFromEnvironmentDisablesLocalEchoWhenRealProviderIsConfigured(t *testing.T) {
	clearGatewayEnvironment(t)
	t.Setenv("OPENAI_API_KEY", "test-key")

	runtime, err := BuildFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if containsModel(runtime.Candidates.Models(), "local/echo") {
		t.Fatalf("local/echo must not be exposed by default beside a real provider: %#v", runtime.Candidates.Models())
	}
	if _, err := runtime.Candidates.Candidates(context.Background(), "local/echo"); !errors.Is(err, core.ErrNoRoute) {
		t.Fatalf("local echo remained routable: %v", err)
	}
	if _, err := runtime.Candidates.Candidates(context.Background(), "openai/test-model"); err != nil {
		t.Fatalf("configured provider direct route failed: %v", err)
	}
}

func TestBuildFromEnvironmentAllowsExplicitLocalEcho(t *testing.T) {
	clearGatewayEnvironment(t)
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("GATEWAY_ENABLE_LOCAL_ECHO", "true")

	runtime, err := BuildFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if !containsModel(runtime.Candidates.Models(), "local/echo") {
		t.Fatalf("explicit local echo was not exposed: %#v", runtime.Candidates.Models())
	}
}

func TestBuildFromEnvironmentRejectsMalformedLocalEchoFlag(t *testing.T) {
	clearGatewayEnvironment(t)
	t.Setenv("GATEWAY_ENABLE_LOCAL_ECHO", "definitely")
	if _, err := BuildFromEnvironment(); err == nil {
		t.Fatal("expected malformed GATEWAY_ENABLE_LOCAL_ECHO to fail")
	}
}

func TestBuildWithCredentialsUsesSystemCredentialForProviderRequests(t *testing.T) {
	clearGatewayEnvironment(t)
	store := credentials.NewMemoryStore()
	ref, err := ProviderCredentialReference("openai")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), ref, []byte("stored-key")); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer stored-key" {
			t.Errorf("authorization=%q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"response"}`))
	}))
	defer server.Close()
	t.Setenv("OPENAI_BASE_URL", server.URL)

	runtime, err := BuildFromEnvironmentWithCredentials(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := runtime.Gateway.Providers.Get("openai")
	if !ok {
		t.Fatal("OpenAI adapter was not registered")
	}
	stream, err := adapter.Execute(context.Background(), provider.AttemptRequest{Request: compiler.EncodedRequest{
		Protocol: canonical.ProtocolOpenAIResponses,
		Body:     []byte(`{"model":"test","input":"hello"}`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	if containsModel(runtime.Candidates.Models(), "local/echo") {
		t.Fatal("stored provider credential did not disable implicit local echo")
	}
}

func TestBuildWithCredentialsKeepsExplicitEnvironmentPrecedence(t *testing.T) {
	clearGatewayEnvironment(t)
	store := credentials.NewMemoryStore()
	ref, err := ProviderCredentialReference("openai")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), ref, []byte("stored-key")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "environment-key")
	value, configured, err := resolveProviderCredential(context.Background(), store, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if !configured || value != "environment-key" {
		t.Fatalf("resolved %q configured=%v", value, configured)
	}
}

func TestProviderCredentialReferenceRejectsUnknownProvider(t *testing.T) {
	if _, err := ProviderCredentialReference("future-provider"); err == nil {
		t.Fatal("accepted unsupported provider credential reference")
	}
}

func clearGatewayEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "GEMINI_API_KEY",
		"DEEPSEEK_API_KEY", "OPENROUTER_API_KEY", "OPENAI_COMPATIBLE_API_KEY", "OPENAI_COMPATIBLE_BASE_URL",
		"GATEWAY_ENABLE_LOCAL_ECHO", "GATEWAY_ROUTES_FILE", "GATEWAY_ROUTES_JSON",
		"GATEWAY_DEFAULT_PROVIDER",
	} {
		t.Setenv(name, "")
	}
}

func containsModel(models []string, wanted string) bool {
	for _, model := range models {
		if model == wanted {
			return true
		}
	}
	return false
}
