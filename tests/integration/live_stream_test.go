package integration_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/stream"
	"github.com/0disoft/relaydock/internal/transport/httpgateway"
)

type stagedStreamingProcessor struct {
	release <-chan struct{}
}

func (p stagedStreamingProcessor) Complete(context.Context, canonical.RequestEnvelope) (httpgateway.Completion, error) {
	panic("streaming path must not fall back to buffered completion")
}

func (p stagedStreamingProcessor) Stream(ctx context.Context, request canonical.RequestEnvelope, hooks httpgateway.StreamHooks) (httpgateway.Completion, error) {
	created := time.Unix(1_700_000_000, 0).UTC()
	if err := hooks.OnCommit(ctx, httpgateway.StreamMetadata{
		RequestID: request.RequestID, Model: request.Model, Provider: "staged", UpstreamModel: "staged-v1", CreatedAt: created,
	}); err != nil {
		return httpgateway.Completion{}, err
	}
	for _, event := range []stream.Event{
		{Kind: stream.EventResponseStarted},
		{Kind: stream.EventContentDelta, Delta: []byte("first")},
	} {
		if err := hooks.OnEvent(ctx, event); err != nil {
			return httpgateway.Completion{}, err
		}
	}
	select {
	case <-ctx.Done():
		return httpgateway.Completion{}, ctx.Err()
	case <-p.release:
	}
	usage := stream.Usage{InputTokens: 3, OutputTokens: 2}
	for _, event := range []stream.Event{
		{Kind: stream.EventContentDelta, Delta: []byte(" second")},
		{Kind: stream.EventUsage, Usage: &usage},
		{Kind: stream.EventCompleted},
	} {
		if err := hooks.OnEvent(ctx, event); err != nil {
			return httpgateway.Completion{}, err
		}
	}
	return httpgateway.Completion{
		ID: request.RequestID, Model: request.Model, UpstreamModel: "staged-v1", Provider: "staged",
		Text: "first second", InputTokens: 3, OutputTokens: 2, CreatedAt: created, Attempts: 2,
	}, nil
}

func TestGatewayFlushesRuntimeDeltaBeforeUpstreamCompletion(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(httpgateway.NewHandlerWithOptions(httpgateway.HandlerOptions{
		Processor: stagedStreamingProcessor{release: release},
		Models:    []string{"code-fast"},
	}))
	defer server.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/responses", strings.NewReader(`{"model":"code-fast","stream":true,"input":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("status=%d body=%s", response.StatusCode, payload)
	}
	if response.Header.Get("X-AI-Runtime-Provider") != "staged" {
		t.Fatalf("route metadata missing: %+v", response.Header)
	}

	reader := bufio.NewReader(response.Body)
	deadline := time.After(2 * time.Second)
	seenFirst := false
	for !seenFirst {
		select {
		case <-deadline:
			t.Fatal("first delta was not flushed before completion was released")
		default:
		}
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatalf("read first delta: %v", readErr)
		}
		seenFirst = strings.Contains(line, `"delta":"first"`)
	}
	close(release)
	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rest), `"delta":" second"`) || !strings.Contains(string(rest), "response.completed") {
		t.Fatalf("terminal stream is incomplete: %s", rest)
	}
	if response.Trailer.Get("X-AI-Runtime-Attempts") != "2" {
		t.Fatalf("attempt trailer=%q", response.Trailer.Get("X-AI-Runtime-Attempts"))
	}
}
