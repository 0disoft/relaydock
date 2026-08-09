package faultinjection_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/defaults"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/stream"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
	"github.com/your-org/ai-runtime-gateway/internal/provider/mock"
	"github.com/your-org/ai-runtime-gateway/internal/routing"
	gatewayruntime "github.com/your-org/ai-runtime-gateway/internal/runtime"
)

func TestClientCancellationPropagatesUpstream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	adapter := mock.New("response that must never be delivered")
	upstream, err := adapter.Execute(ctx, provider.AttemptRequest{})
	if err != nil {
		t.Fatalf("create upstream stream: %v", err)
	}
	defer upstream.Close()
	cancel()

	if _, err := upstream.Next(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation did not propagate to upstream stream: %v", err)
	}
}

func TestPartialStreamIsNotMarkedCompleted(t *testing.T) {
	adapter := &scriptedAdapter{events: []stream.Event{
		{Kind: stream.EventResponseStarted},
		{Kind: stream.EventToolCallDelta, ItemID: "dangerous_call", Delta: []byte(`{"path":"/tmp/a"}`)},
	}}
	providers := provider.NewRegistry()
	providers.Register(adapter)
	candidates := gatewayruntime.NewMemoryCandidateSource()
	candidates.Set("code-deep", []routing.Candidate{{
		Provider:     adapter.Name(),
		AccountID:    "account_1",
		Model:        "code-deep",
		Protocol:     canonical.ProtocolOpenAIResponses,
		Capabilities: canonical.CapabilitySet{canonical.CapabilityTextInput: true},
		HealthScore:  1,
		Available:    true,
	}})
	gateway := gatewayruntime.NewGateway()
	gateway.Compiler = defaults.Compiler()
	gateway.Router = routing.New(nil)
	gateway.Providers = providers
	gateway.Candidates = candidates
	gateway.MaximumAttempts = 1

	seenCompleted := false
	_, err := gateway.Run(context.Background(), canonical.RequestEnvelope{
		RequestID:       "request_1",
		IngressProtocol: canonical.ProtocolOpenAIResponses,
		Model:           "code-deep",
		Items: []canonical.Item{{
			ID:      "item_1",
			Kind:    canonical.ItemMessage,
			Role:    canonical.RoleUser,
			Content: []canonical.ContentPart{{Type: "text", Text: "inspect this"}},
		}},
		Requirements: canonical.CapabilityRequirements{Required: []canonical.Capability{canonical.CapabilityTextInput}},
	}, func(_ context.Context, event stream.Event) error {
		if event.Kind == stream.EventCompleted {
			seenCompleted = true
		}
		return nil
	})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("partial stream returned %v, want unexpected EOF", err)
	}
	if seenCompleted {
		t.Fatal("gateway synthesized completion after a partial tool call")
	}
}

type scriptedAdapter struct {
	events []stream.Event
}

func (*scriptedAdapter) Name() string { return "partial-provider" }
func (*scriptedAdapter) Capabilities(string) canonical.CapabilitySet {
	return canonical.CapabilitySet{canonical.CapabilityTextInput: true}
}
func (a *scriptedAdapter) Execute(context.Context, provider.AttemptRequest) (provider.AttemptStream, error) {
	return &scriptedStream{events: append([]stream.Event(nil), a.events...)}, nil
}
func (*scriptedAdapter) ParseUsage(context.Context, provider.AttemptResult) (stream.Usage, error) {
	return stream.Usage{}, nil
}

type scriptedStream struct {
	mu     sync.Mutex
	events []stream.Event
	closed bool
}

func (s *scriptedStream) Next(ctx context.Context) (stream.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return stream.Event{}, err
	}
	if s.closed || len(s.events) == 0 {
		return stream.Event{}, io.EOF
	}
	event := s.events[0]
	s.events = s.events[1:]
	return event, nil
}

func (s *scriptedStream) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}
