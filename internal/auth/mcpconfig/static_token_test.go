package mcpconfig

import (
	"errors"
	"testing"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

func TestStaticTokenPrefersExplicitCredential(t *testing.T) {
	actual, err := StaticToken("  mcp-token  ", "broker-token", "false", true)
	if err != nil {
		t.Fatal(err)
	}
	if actual != "mcp-token" {
		t.Fatalf("expected explicit token, got %q", actual)
	}
}

func TestStaticTokenDoesNotReuseBrokerWhenScopedTokensAreConfigured(t *testing.T) {
	actual, err := StaticToken("", "broker-token", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if actual != "" {
		t.Fatalf("expected no legacy credential, got %q", actual)
	}
}

func TestStaticTokenKeepsLegacyFallbackWithoutScopedTokens(t *testing.T) {
	actual, err := StaticToken("", " broker-token ", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if actual != "broker-token" {
		t.Fatalf("expected broker fallback, got %q", actual)
	}
}

func TestStaticTokenAllowsExplicitBrokerReuse(t *testing.T) {
	actual, err := StaticToken("", "broker-token", "true", true)
	if err != nil {
		t.Fatal(err)
	}
	if actual != "broker-token" {
		t.Fatalf("expected explicit broker reuse, got %q", actual)
	}
}

func TestStaticTokenRejectsMalformedBoolean(t *testing.T) {
	_, err := StaticToken("", "broker-token", "sometimes", true)
	if !errors.Is(err, core.ErrInvalidConfiguration) {
		t.Fatalf("expected invalid configuration, got %v", err)
	}
}
