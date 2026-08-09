package faultinjection_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/compiler"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/stream"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
	"github.com/your-org/ai-runtime-gateway/internal/provider/httpadapter"
)

func TestHTTPAdapterDoesNotSynthesizeCompletionAtSSEEOF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.created\"}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
	}))
	defer server.Close()

	upstream := newHTTPTestStream(t, server.URL)
	defer upstream.Close()

	first, err := upstream.Next(context.Background())
	if err != nil || first.Kind != stream.EventResponseStarted {
		t.Fatalf("first event=%+v err=%v", first, err)
	}
	second, err := upstream.Next(context.Background())
	if err != nil || second.Kind != stream.EventContentDelta || string(second.Delta) != "partial" {
		t.Fatalf("second event=%+v err=%v", second, err)
	}
	if event, err := upstream.Next(context.Background()); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("EOF event=%+v err=%v, want unexpected EOF", event, err)
	}
}

func TestHTTPAdapterAcceptsExplicitDoneAndMultilineSSEData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: response.output_text.delta\n")
		_, _ = io.WriteString(w, "data: {\"delta\":\n")
		_, _ = io.WriteString(w, "data: \"hello\"}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	upstream := newHTTPTestStream(t, server.URL)
	defer upstream.Close()

	first, err := upstream.Next(context.Background())
	if err != nil || first.Kind != stream.EventContentDelta || string(first.Delta) != "hello" {
		t.Fatalf("first event=%+v err=%v", first, err)
	}
	second, err := upstream.Next(context.Background())
	if err != nil || second.Kind != stream.EventCompleted {
		t.Fatalf("second event=%+v err=%v", second, err)
	}
	if _, err := upstream.Next(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("terminal read err=%v, want EOF", err)
	}
}

func newHTTPTestStream(t *testing.T, baseURL string) provider.AttemptStream {
	t.Helper()
	adapter := httpadapter.New(httpadapter.Config{
		Name:         "fixture",
		BaseURL:      baseURL,
		APIKey:       "fixture-key",
		Paths:        map[canonical.Protocol]string{canonical.ProtocolOpenAIResponses: "/stream"},
		Capabilities: canonical.CapabilitySet{canonical.CapabilityTextInput: true},
	})
	upstream, err := adapter.Execute(context.Background(), provider.AttemptRequest{
		Model: "fixture-model",
		Request: compiler.EncodedRequest{
			Protocol: canonical.ProtocolOpenAIResponses,
			Body:     []byte(`{"model":"fixture-model","input":"hello","stream":true}`),
		},
	})
	if err != nil {
		t.Fatalf("execute adapter: %v", err)
	}
	return upstream
}
