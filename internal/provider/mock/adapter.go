package mock

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/stream"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
)

type Adapter struct{ Text string }

func New(text string) *Adapter {
	if text == "" {
		text = "Mock provider is active."
	}
	return &Adapter{Text: text}
}
func (*Adapter) Name() string { return "mock" }
func (*Adapter) Capabilities(string) canonical.CapabilitySet {
	return canonical.CapabilitySet{canonical.CapabilityTextInput: true, canonical.CapabilityToolCalling: true, canonical.CapabilityStructuredOutput: true, canonical.CapabilityReasoning: true}
}
func (a *Adapter) Execute(context.Context, provider.AttemptRequest) (provider.AttemptStream, error) {
	text := a.Text
	return &eventStream{events: []stream.Event{{Kind: stream.EventResponseStarted}, {Kind: stream.EventContentDelta, Delta: []byte(text)}, {Kind: stream.EventUsage, Usage: &stream.Usage{InputTokens: 16, OutputTokens: int64(len(strings.Fields(text))) + 1}}, {Kind: stream.EventCompleted}}}, nil
}
func (*Adapter) ParseUsage(context.Context, provider.AttemptResult) (stream.Usage, error) {
	return stream.Usage{}, nil
}

type eventStream struct {
	mu     sync.Mutex
	events []stream.Event
	closed bool
}

func (s *eventStream) Next(ctx context.Context) (stream.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return stream.Event{}, err
	}
	if s.closed || len(s.events) == 0 {
		return stream.Event{}, io.EOF
	}
	e := s.events[0]
	s.events = s.events[1:]
	return e, nil
}
func (s *eventStream) Close() error { s.mu.Lock(); s.closed = true; s.mu.Unlock(); return nil }
