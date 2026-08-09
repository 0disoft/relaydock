package conformance_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/compiler"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
	"github.com/your-org/ai-runtime-gateway/internal/provider/httpadapter"
)

func TestOversizedProviderResponseIsRejectedWithoutTruncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"` + strings.Repeat("x", 256) + `"}`))
	}))
	defer server.Close()

	adapter := httpadapter.New(httpadapter.Config{
		Name:                 "oversized",
		BaseURL:              server.URL,
		APIKey:               "test-key",
		Paths:                map[canonical.Protocol]string{canonical.ProtocolOpenAIResponses: "/v1/responses"},
		MaximumResponseBytes: 64,
	})
	upstream, err := adapter.Execute(context.Background(), provider.AttemptRequest{Request: compiler.EncodedRequest{
		Protocol: canonical.ProtocolOpenAIResponses,
		Body:     []byte(`{"model":"test","input":"hello"}`),
	}})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	defer upstream.Close()
	if _, err := upstream.Next(context.Background()); !errors.Is(err, core.ErrFrameTooLarge) {
		t.Fatalf("oversized response returned %v, want frame-too-large", err)
	}
}
