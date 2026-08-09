package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
	"github.com/your-org/ai-runtime-gateway/internal/provider/mock"
	"github.com/your-org/ai-runtime-gateway/internal/routing"
)

func TestDecodeRouteConfigRejectsTrailingGarbageAndUnknownFields(t *testing.T) {
	for _, raw := range []string{
		`{"code-fast":[{"provider":"mock","model":"local/echo"}]} trailing`,
		`{"code-fast":[{"provider":"mock","model":"local/echo"}]} {"second":[]}`,
		`{"code-fast":[{"provider":"mock","model":"local/echo","unknown":true}]}`,
	} {
		if _, err := decodeRouteConfig([]byte(raw)); err == nil {
			t.Fatalf("expected invalid route JSON to fail: %s", raw)
		}
	}
}

func TestCandidateSourceSupportsVirtualAndConfiguredDirectRoutes(t *testing.T) {
	adapter := mock.New("ok")
	definitions := map[string]providerDefinition{
		"mock": {Adapter: adapter, Protocol: canonical.ProtocolOpenAIResponses, Configured: true},
	}
	source, err := newCandidateSource(definitions, map[string][]RouteConfig{
		"code-fast": {{Provider: "mock", Model: "upstream-fast", ConcurrencyLimit: 3}},
	}, "mock")
	if err != nil {
		t.Fatal(err)
	}

	virtual, err := source.Candidates(context.Background(), "code-fast")
	if err != nil || len(virtual) != 1 {
		t.Fatalf("virtual route=%+v err=%v", virtual, err)
	}
	if virtual[0].Model != "upstream-fast" || virtual[0].ConcurrencyLimit != 3 {
		t.Fatalf("unexpected virtual candidate: %+v", virtual[0])
	}

	direct, err := source.Candidates(context.Background(), "another-model")
	if err != nil || len(direct) != 1 {
		t.Fatalf("default provider route=%+v err=%v", direct, err)
	}
	if direct[0].Provider != "mock" || direct[0].Model != "another-model" {
		t.Fatalf("unexpected direct candidate: %+v", direct[0])
	}

	if _, err := source.Candidates(context.Background(), "missing/provider-model"); !errors.Is(err, core.ErrNoRoute) {
		t.Fatalf("unconfigured provider must not be routable: %v", err)
	}
}

func TestCandidateSourceRejectsUnconfiguredProvider(t *testing.T) {
	adapter := mock.New("ok")
	_, err := newCandidateSource(map[string]providerDefinition{
		"mock": {Adapter: adapter, Protocol: canonical.ProtocolOpenAIResponses, Configured: false},
	}, map[string][]RouteConfig{
		"code-fast": {{Provider: "mock", Model: "upstream-fast"}},
	}, "")
	if !errors.Is(err, core.ErrInvalidConfiguration) {
		t.Fatalf("expected invalid configuration, got %v", err)
	}
}

func TestCandidateSourceAppliesCooldownAcrossRequests(t *testing.T) {
	adapter := mock.New("ok")
	source, err := newCandidateSource(map[string]providerDefinition{
		"mock": {Adapter: adapter, Protocol: canonical.ProtocolOpenAIResponses, Configured: true},
	}, map[string][]RouteConfig{
		"code-fast": {{Provider: "mock", AccountID: "mock/account", Model: "upstream-fast"}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	source.now = func() time.Time { return now }
	candidates, err := source.Candidates(context.Background(), "code-fast")
	if err != nil {
		t.Fatal(err)
	}
	candidate := candidates[0]
	if err := source.RecordOutcome(context.Background(), routing.Outcome{
		Candidate: candidate,
		Success:   false,
		ErrorCode: string(provider.ErrorClassRateLimit),
	}); err != nil {
		t.Fatal(err)
	}
	cooled, err := source.Candidates(context.Background(), "code-fast")
	if err != nil {
		t.Fatal(err)
	}
	if cooled[0].Available || cooled[0].HealthScore != 0 {
		t.Fatalf("candidate was not cooled down: %+v", cooled[0])
	}
	now = now.Add(31 * time.Second)
	recovered, err := source.Candidates(context.Background(), "code-fast")
	if err != nil {
		t.Fatal(err)
	}
	if !recovered[0].Available || recovered[0].HealthScore <= 0 || recovered[0].HealthScore >= candidate.HealthScore {
		t.Fatalf("candidate did not recover with a health penalty: before=%+v after=%+v", candidate, recovered[0])
	}
	if err := source.RecordOutcome(context.Background(), routing.Outcome{Candidate: candidate, Success: true}); err != nil {
		t.Fatal(err)
	}
	healthy, err := source.Candidates(context.Background(), "code-fast")
	if err != nil {
		t.Fatal(err)
	}
	if !healthy[0].Available || healthy[0].HealthScore <= recovered[0].HealthScore {
		t.Fatalf("success did not heal candidate score: recovered=%+v healthy=%+v", recovered[0], healthy[0])
	}
}

func TestCancelledOutcomeDoesNotPenalizeProvider(t *testing.T) {
	adapter := mock.New("ok")
	source, err := newCandidateSource(map[string]providerDefinition{
		"mock": {Adapter: adapter, Protocol: canonical.ProtocolOpenAIResponses, Configured: true},
	}, map[string][]RouteConfig{
		"code-fast": {{Provider: "mock", AccountID: "mock/account", Model: "upstream-fast"}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := source.Candidates(context.Background(), "code-fast")
	if err != nil {
		t.Fatal(err)
	}
	if err := source.RecordOutcome(context.Background(), routing.Outcome{
		Candidate: candidate[0], ErrorCode: string(provider.ErrorClassCancelled),
	}); err != nil {
		t.Fatal(err)
	}
	after, err := source.Candidates(context.Background(), "code-fast")
	if err != nil {
		t.Fatal(err)
	}
	if !after[0].Available || after[0].HealthScore != candidate[0].HealthScore {
		t.Fatalf("client cancellation penalized provider: before=%+v after=%+v", candidate[0], after[0])
	}
}

func TestCandidateSourceCanDisableDirectModelBypass(t *testing.T) {
	adapter := mock.New("ok")
	source, err := newCandidateSource(map[string]providerDefinition{
		"mock": {Adapter: adapter, Protocol: canonical.ProtocolOpenAIResponses, Configured: true},
	}, map[string][]RouteConfig{
		"code-fast": {{Provider: "mock", Model: "upstream-fast"}},
	}, "mock")
	if err != nil {
		t.Fatal(err)
	}
	source.SetAllowDirectRouting(false)

	if _, err := source.Candidates(context.Background(), "code-fast"); err != nil {
		t.Fatalf("signed virtual route must remain available: %v", err)
	}
	for _, model := range []string{"unlisted-model", "mock/unlisted-model"} {
		if _, err := source.Candidates(context.Background(), model); !errors.Is(err, core.ErrNoRoute) {
			t.Fatalf("direct model %q bypassed route policy: %v", model, err)
		}
	}
	if status := source.SnapshotStatus(); status.AllowDirectRouting {
		t.Fatalf("snapshot status did not expose disabled direct routing: %#v", status)
	}
}

func TestCandidateRuntimeStateIsBounded(t *testing.T) {
	adapter := mock.New("ok")
	source, err := newCandidateSource(map[string]providerDefinition{
		"mock": {Adapter: adapter, Protocol: canonical.ProtocolOpenAIResponses, Configured: true},
	}, nil, "mock")
	if err != nil {
		t.Fatal(err)
	}
	source.maximumRuntimeStates = 2
	now := time.Date(2026, 8, 8, 1, 0, 0, 0, time.UTC)
	source.now = func() time.Time { return now }

	for index, model := range []string{"model-a", "model-b", "model-c"} {
		candidate, err := source.Candidates(context.Background(), model)
		if err != nil {
			t.Fatal(err)
		}
		if err := source.RecordOutcome(context.Background(), routing.Outcome{
			Candidate: candidate[0], ErrorCode: string(provider.ErrorClassOverloaded),
		}); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Second)
		_ = index
	}

	source.mu.Lock()
	defer source.mu.Unlock()
	if len(source.states) != 2 {
		t.Fatalf("runtime state count = %d, want 2", len(source.states))
	}
	if _, exists := source.states["mock/mock/default/model-a"]; exists {
		t.Fatalf("oldest runtime state was not evicted: %#v", source.states)
	}
}
