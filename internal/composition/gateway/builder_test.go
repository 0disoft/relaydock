package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/your-org/ai-runtime-gateway/internal/core"
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

func clearGatewayEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "GEMINI_API_KEY",
		"DEEPSEEK_API_KEY", "OPENROUTER_API_KEY", "OPENAI_COMPATIBLE_BASE_URL",
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
