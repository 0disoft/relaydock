package conformance_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/provider"
)

func TestProviderHTTPErrorClassificationAndRetryAfter(t *testing.T) {
	now := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	header := make(http.Header)
	header.Set("Retry-After", "3")
	err := provider.NewHTTPError("openai", http.StatusTooManyRequests, `{"error":"rate limit"}`, header, now)
	if err.Class != provider.ErrorClassRateLimit || !err.Retryable() {
		t.Fatalf("class=%q retryable=%v", err.Class, err.Retryable())
	}
	if err.RetryAfter != 3*time.Second {
		t.Fatalf("retry after=%s, want 3s", err.RetryAfter)
	}
	if !provider.IsRetryable(err) {
		t.Fatal("429 must be retryable")
	}
}

func TestProviderAuthenticationErrorIsNotRetryable(t *testing.T) {
	err := provider.NewHTTPError("anthropic", http.StatusUnauthorized, "invalid key", nil, time.Now())
	if err.Class != provider.ErrorClassAuthentication {
		t.Fatalf("class=%q, want authentication", err.Class)
	}
	if provider.IsRetryable(err) {
		t.Fatal("authentication failure must not be retried")
	}
}

func TestTransportErrorPreservesCause(t *testing.T) {
	cause := errors.New("dial failed")
	err := provider.WrapTransportError("google", cause)
	if !errors.Is(err, cause) {
		t.Fatal("transport error did not preserve cause")
	}
	if !provider.IsRetryable(err) {
		t.Fatal("transport failure must be retryable")
	}
}
