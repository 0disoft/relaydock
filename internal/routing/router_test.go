package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
)

func TestUnavailableCandidateIsNeverSelected(t *testing.T) {
	router := New(nil)
	_, err := router.Select(context.Background(), Request{}, []Candidate{{
		Provider:     "degraded",
		Model:        "model",
		Available:    false,
		HealthScore:  1,
		Capabilities: canonical.CapabilitySet{},
	}})
	if !errors.Is(err, core.ErrNoRoute) {
		t.Fatalf("expected unavailable candidate to be rejected, got %v", err)
	}
}

func TestTieBreakIsDeterministic(t *testing.T) {
	router := New(nil)
	candidates := []Candidate{
		{Provider: "z", AccountID: "a", Model: "m", Available: true, HealthScore: 1, Capabilities: canonical.CapabilitySet{}},
		{Provider: "a", AccountID: "a", Model: "m", Available: true, HealthScore: 1, Capabilities: canonical.CapabilitySet{}},
	}
	decision, err := router.Select(context.Background(), Request{}, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Candidate.Provider != "a" {
		t.Fatalf("provider = %q, want deterministic a", decision.Candidate.Provider)
	}
}
