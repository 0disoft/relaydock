package faultinjection_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/defaults"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/stream"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
	"github.com/your-org/ai-runtime-gateway/internal/routing"
	gatewayruntime "github.com/your-org/ai-runtime-gateway/internal/runtime"
)

func TestPreSemanticFailureDoesNotLeakAttemptEvents(t *testing.T) {
	first := &sequenceAdapter{name: "a-first", streams: [][]stream.Event{{
		{Kind: stream.EventResponseStarted},
	}}}
	second := &sequenceAdapter{name: "b-second", streams: [][]stream.Event{{
		{Kind: stream.EventResponseStarted},
		{Kind: stream.EventContentDelta, Delta: []byte("final")},
		{Kind: stream.EventCompleted},
	}}}
	gateway := testGateway(t, first, second)
	gateway.MaximumAttempts = 2
	gateway.RetryBaseDelay = 0

	var events []stream.Event
	var committed []string
	result, err := gateway.RunWithHooks(context.Background(), testEnvelope(), gatewayruntime.RunHooks{
		OnCommit: func(_ context.Context, decision routing.Decision) error {
			committed = append(committed, decision.Candidate.Provider)
			return nil
		},
		OnEvent: func(_ context.Context, event stream.Event) error {
			events = append(events, event)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run gateway: %v", err)
	}
	if result.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", result.Attempts)
	}
	if len(committed) != 1 || committed[0] != second.name {
		t.Fatalf("committed routes = %#v, want only %q", committed, second.name)
	}
	started := 0
	for _, event := range events {
		if event.Kind == stream.EventResponseStarted {
			started++
		}
	}
	if started != 1 {
		t.Fatalf("response.started events = %d, want 1; leaked failed attempt: %#v", started, events)
	}
	if len(events) != 3 || string(events[1].Delta) != "final" || events[2].Kind != stream.EventCompleted {
		t.Fatalf("unexpected delivered events: %#v", events)
	}
	if !errors.Is(errors.Unwrap(&provider.UpstreamError{Cause: io.ErrUnexpectedEOF}), io.ErrUnexpectedEOF) {
		t.Fatal("provider error must preserve its cause")
	}
}

func TestNonRetryableProviderErrorStopsImmediately(t *testing.T) {
	first := &sequenceAdapter{
		name: "a-first",
		executeErrors: []error{&provider.UpstreamError{
			Provider:   "a-first",
			Class:      provider.ErrorClassInvalidRequest,
			StatusCode: 400,
			Message:    "bad request",
		}},
	}
	second := &sequenceAdapter{name: "b-second", streams: [][]stream.Event{{
		{Kind: stream.EventResponseStarted},
		{Kind: stream.EventContentDelta, Delta: []byte("must not run")},
		{Kind: stream.EventCompleted},
	}}}
	gateway := testGateway(t, first, second)
	gateway.MaximumAttempts = 2
	gateway.RetryBaseDelay = 0

	result, err := gateway.Run(context.Background(), testEnvelope(), nil)
	if err == nil {
		t.Fatal("expected non-retryable provider error")
	}
	if provider.ErrorCode(err) != string(provider.ErrorClassInvalidRequest) {
		t.Fatalf("error code = %q, want invalid_request", provider.ErrorCode(err))
	}
	if result.Attempts != 1 || second.executeCount() != 0 {
		t.Fatalf("attempts=%d second executions=%d, want 1 and 0", result.Attempts, second.executeCount())
	}
}

func TestLeaseIsReleasedAfterCommittedAttempt(t *testing.T) {
	adapter := &sequenceAdapter{name: "provider", streams: [][]stream.Event{{
		{Kind: stream.EventResponseStarted},
		{Kind: stream.EventContentDelta, Delta: []byte("ok")},
		{Kind: stream.EventCompleted},
	}}}
	gateway := testGateway(t, adapter)
	leases := &trackingLeaseManager{}
	gateway.Leases = leases
	gateway.LeaseTTL = time.Minute

	if _, err := gateway.Run(context.Background(), testEnvelope(), nil); err != nil {
		t.Fatalf("run gateway: %v", err)
	}
	if leases.acquired != 1 || leases.released != 1 {
		t.Fatalf("lease acquired=%d released=%d, want 1/1", leases.acquired, leases.released)
	}
}

func testGateway(t *testing.T, adapters ...provider.Adapter) *gatewayruntime.Gateway {
	t.Helper()
	registry := provider.NewRegistry()
	source := gatewayruntime.NewMemoryCandidateSource()
	candidates := make([]routing.Candidate, 0, len(adapters))
	for _, adapter := range adapters {
		registry.Register(adapter)
		candidates = append(candidates, routing.Candidate{
			Provider:     adapter.Name(),
			AccountID:    adapter.Name() + "-account",
			Model:        "upstream-model",
			Protocol:     canonical.ProtocolOpenAIResponses,
			Capabilities: canonical.CapabilitySet{canonical.CapabilityTextInput: true},
			HealthScore:  1,
			Available:    true,
		})
	}
	source.Set("code-deep", candidates)
	gateway := gatewayruntime.NewGateway()
	gateway.Compiler = defaults.Compiler()
	gateway.Router = routing.New(nil)
	gateway.Providers = registry
	gateway.Candidates = source
	return gateway
}

func testEnvelope() canonical.RequestEnvelope {
	return canonical.RequestEnvelope{
		RequestID:       "request_1",
		IngressProtocol: canonical.ProtocolOpenAIResponses,
		Model:           "code-deep",
		Items: []canonical.Item{{
			ID:      "item_1",
			Kind:    canonical.ItemMessage,
			Role:    canonical.RoleUser,
			Content: []canonical.ContentPart{{Type: "text", Text: "solve this"}},
		}},
		Requirements: canonical.CapabilityRequirements{Required: []canonical.Capability{canonical.CapabilityTextInput}},
	}
}

type sequenceAdapter struct {
	name          string
	mu            sync.Mutex
	streams       [][]stream.Event
	executeErrors []error
	executions    int
}

func (a *sequenceAdapter) Name() string { return a.name }
func (a *sequenceAdapter) Capabilities(string) canonical.CapabilitySet {
	return canonical.CapabilitySet{canonical.CapabilityTextInput: true}
}
func (a *sequenceAdapter) Execute(context.Context, provider.AttemptRequest) (provider.AttemptStream, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	index := a.executions
	a.executions++
	if index < len(a.executeErrors) && a.executeErrors[index] != nil {
		return nil, a.executeErrors[index]
	}
	if index >= len(a.streams) {
		return &sequenceStream{}, nil
	}
	return &sequenceStream{events: append([]stream.Event(nil), a.streams[index]...)}, nil
}
func (*sequenceAdapter) ParseUsage(context.Context, provider.AttemptResult) (stream.Usage, error) {
	return stream.Usage{}, nil
}
func (a *sequenceAdapter) executeCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.executions
}

type sequenceStream struct {
	mu     sync.Mutex
	events []stream.Event
	closed bool
}

func (s *sequenceStream) Next(ctx context.Context) (stream.Event, error) {
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
func (s *sequenceStream) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}

type trackingLeaseManager struct {
	mu       sync.Mutex
	acquired int
	released int
}

func (m *trackingLeaseManager) Acquire(context.Context, routing.Candidate, time.Duration) (routing.Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.acquired++
	return routing.Lease{ID: "lease", AccountID: "account", ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (m *trackingLeaseManager) Renew(_ context.Context, lease routing.Lease, ttl time.Duration) (routing.Lease, error) {
	lease.ExpiresAt = time.Now().Add(ttl)
	return lease, nil
}
func (m *trackingLeaseManager) Release(context.Context, routing.Lease) error {
	m.mu.Lock()
	m.released++
	m.mu.Unlock()
	return nil
}

func TestAttemptHooksObserveEveryAttempt(t *testing.T) {
	first := &sequenceAdapter{name: "a-first", streams: [][]stream.Event{{
		{Kind: stream.EventResponseStarted},
	}}}
	second := &sequenceAdapter{name: "b-second", streams: [][]stream.Event{{
		{Kind: stream.EventResponseStarted},
		{Kind: stream.EventContentDelta, Delta: []byte("ok")},
		{Kind: stream.EventCompleted},
	}}}
	gateway := testGateway(t, first, second)
	gateway.MaximumAttempts = 2
	gateway.RetryBaseDelay = 0
	var started []gatewayruntime.AttemptSummary
	var completed []gatewayruntime.AttemptSummary
	_, err := gateway.RunWithHooks(context.Background(), testEnvelope(), gatewayruntime.RunHooks{
		OnAttemptStarted: func(_ context.Context, summary gatewayruntime.AttemptSummary) error {
			started = append(started, summary)
			return nil
		},
		OnAttemptCompleted: func(_ context.Context, summary gatewayruntime.AttemptSummary, _ error) error {
			completed = append(completed, summary)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(started) != 2 || len(completed) != 2 {
		t.Fatalf("started=%d completed=%d", len(started), len(completed))
	}
	if completed[0].Committed || !completed[1].Committed || completed[0].ErrorCode == "" {
		t.Fatalf("unexpected completed attempts: %+v", completed)
	}
}

func TestAttemptStartHookFailureReleasesLease(t *testing.T) {
	adapter := &sequenceAdapter{name: "provider", streams: [][]stream.Event{{
		{Kind: stream.EventResponseStarted},
		{Kind: stream.EventContentDelta, Delta: []byte("must-not-run")},
		{Kind: stream.EventCompleted},
	}}}
	gateway := testGateway(t, adapter)
	leases := &trackingLeaseManager{}
	gateway.Leases = leases
	expected := errors.New("journal unavailable")
	_, err := gateway.RunWithHooks(context.Background(), testEnvelope(), gatewayruntime.RunHooks{
		OnAttemptStarted: func(context.Context, gatewayruntime.AttemptSummary) error { return expected },
	})
	if !errors.Is(err, expected) {
		t.Fatalf("error=%v", err)
	}
	if leases.acquired != 1 || leases.released != 1 || adapter.executeCount() != 0 {
		t.Fatalf("acquired=%d released=%d executions=%d", leases.acquired, leases.released, adapter.executeCount())
	}
}

func TestCommitHookFailureCrossesRetryBoundary(t *testing.T) {
	first := &sequenceAdapter{name: "a-first", streams: [][]stream.Event{{
		{Kind: stream.EventResponseStarted},
		{Kind: stream.EventContentDelta, Delta: []byte("semantic")},
		{Kind: stream.EventCompleted},
	}}}
	second := &sequenceAdapter{name: "b-second", streams: [][]stream.Event{{
		{Kind: stream.EventContentDelta, Delta: []byte("must-not-run")},
		{Kind: stream.EventCompleted},
	}}}
	gateway := testGateway(t, first, second)
	gateway.MaximumAttempts = 2
	expected := errors.New("downstream disconnected")
	result, err := gateway.RunWithHooks(context.Background(), testEnvelope(), gatewayruntime.RunHooks{
		OnCommit: func(context.Context, routing.Decision) error { return expected },
	})
	if !errors.Is(err, expected) {
		t.Fatalf("error=%v", err)
	}
	if result.Attempts != 1 || len(result.AttemptSummaries) != 1 || !result.AttemptSummaries[0].Committed {
		t.Fatalf("result=%+v", result)
	}
	if second.executeCount() != 0 {
		t.Fatalf("second provider executed %d times", second.executeCount())
	}
}
