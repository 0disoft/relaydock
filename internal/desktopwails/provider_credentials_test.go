package desktopwails

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	gatewaycomposition "github.com/0disoft/relaydock/internal/composition/gateway"
	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/credentials"
)

func TestRuntimeServiceManagesProviderCredentialWithoutReturningSecret(t *testing.T) {
	clearProviderCredentialEnvironment(t)
	store := credentials.NewMemoryStore()
	service := NewRuntimeServiceWithCredentialStore(nil, store, nil)
	openAI := providerSummary(t, service.ListProviders(), "openai")
	if openAI.Configured || !openAI.CredentialWritable || openAI.CredentialSource != "none" {
		t.Fatalf("unexpected initial provider summary: %#v", openAI)
	}
	secret := "sk-system-example"
	if err := service.SaveProviderCredential("openai", secret); err != nil {
		t.Fatal(err)
	}
	openAI = providerSummary(t, service.ListProviders(), "openai")
	if !openAI.Configured || !openAI.CredentialWritable || openAI.CredentialSource != "system" {
		t.Fatalf("unexpected stored provider summary: %#v", openAI)
	}
	raw, err := json.Marshal(service.ListProviders())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("provider summary returned credential material")
	}
	if err := service.DeleteProviderCredential("openai"); err != nil {
		t.Fatal(err)
	}
	if providerSummary(t, service.ListProviders(), "openai").Configured {
		t.Fatal("provider remained configured after credential deletion")
	}
}

func TestRuntimeServiceRejectsCredentialChangesDuringGatewayOrEnvironmentOverride(t *testing.T) {
	clearProviderCredentialEnvironment(t)
	service := NewRuntimeServiceWithCredentialStore(nil, credentials.NewMemoryStore(), nil)
	service.mu.Lock()
	service.gatewayStarting = true
	service.mu.Unlock()
	if err := service.SaveProviderCredential("openai", "secret"); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("gateway-starting write error=%v", err)
	}
	service.mu.Lock()
	service.gatewayStarting = false
	service.mu.Unlock()
	t.Setenv("OPENAI_API_KEY", "environment-key")
	if err := service.SaveProviderCredential("openai", "secret"); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("environment override write error=%v", err)
	}
	if summary := providerSummary(t, service.ListProviders(), "openai"); !summary.Configured || summary.CredentialWritable || summary.CredentialSource != "environment" {
		t.Fatalf("unexpected environment provider summary: %#v", summary)
	}
}

func TestRuntimeServiceReportsUnavailableCredentialStoreWithoutFallback(t *testing.T) {
	clearProviderCredentialEnvironment(t)
	service := NewRuntimeServiceWithCredentialStore(nil, nil, errors.New("native store unavailable"))
	if err := service.SaveProviderCredential("openai", "secret"); !errors.Is(err, credentials.ErrSystemStoreUnavailable) {
		t.Fatalf("unavailable store error=%v", err)
	}
	if summary := providerSummary(t, service.ListProviders(), "openai"); summary.CredentialWritable || summary.CredentialSource != "unavailable" {
		t.Fatalf("unexpected unavailable provider summary: %#v", summary)
	}
}

func TestRuntimeServiceRejectsUnknownOrWhitespaceCredential(t *testing.T) {
	clearProviderCredentialEnvironment(t)
	service := NewRuntimeServiceWithCredentialStore(nil, credentials.NewMemoryStore(), nil)
	if err := service.SaveProviderCredential("future", "secret"); !errors.Is(err, core.ErrInvalidArgument) {
		t.Fatalf("unknown provider error=%v", err)
	}
	if err := service.SaveProviderCredential("openai", " secret "); !errors.Is(err, core.ErrInvalidArgument) {
		t.Fatalf("whitespace credential error=%v", err)
	}
	if err := service.SaveProviderCredential("openai", strings.Repeat("x", credentials.MaximumSystemCredentialBytes+1)); !errors.Is(err, core.ErrInvalidArgument) {
		t.Fatalf("oversized credential error=%v, want ErrInvalidArgument", err)
	}
}

func providerSummary(t *testing.T, summaries []ProviderSummary, providerID string) ProviderSummary {
	t.Helper()
	for _, summary := range summaries {
		if summary.ID == providerID {
			return summary
		}
	}
	t.Fatalf("provider %q not found", providerID)
	return ProviderSummary{}
}

func clearProviderCredentialEnvironment(t *testing.T) {
	t.Helper()
	for _, providerID := range []string{"openai", "anthropic", "google", "deepseek", "openrouter", "openai-compatible"} {
		ref, err := gatewaycomposition.ProviderCredentialReference(providerID)
		if err != nil || ref.Provider == "" {
			t.Fatalf("invalid provider definition %q: %v", providerID, err)
		}
	}
	for _, name := range []string{
		"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "GEMINI_API_KEY",
		"DEEPSEEK_API_KEY", "OPENROUTER_API_KEY", "OPENAI_COMPATIBLE_API_KEY", "OPENAI_COMPATIBLE_BASE_URL",
	} {
		t.Setenv(name, "")
	}
}
