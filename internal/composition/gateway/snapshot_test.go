package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/control/snapshot"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/provider/mock"
)

func TestCandidateSourceAppliesSignedSnapshotRoutesAtomically(t *testing.T) {
	t.Parallel()
	definitions := map[string]providerDefinition{
		"mock": {Adapter: mock.New("ok"), Protocol: canonical.ProtocolOpenAIResponses, Configured: true},
	}
	source, err := newCandidateSource(definitions, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	source.SetRequireFreshSnapshot(true)
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	source.now = func() time.Time { return now }
	value := snapshot.Snapshot{
		Revision:    12,
		GeneratedAt: now,
		ExpiresAt:   now.Add(time.Hour),
		Models: []snapshot.ModelRoute{{
			VirtualModel: "code-fast",
			CandidateDetails: []snapshot.RouteCandidate{{
				Provider: "mock", Model: "local/echo", Protocol: string(canonical.ProtocolOpenAIResponses), ConcurrencyLimit: 4,
			}},
		}},
		PriceRevisions: []snapshot.PriceRevision{
			{ID: "price-old", EffectiveAt: now.Add(-time.Hour)},
			{ID: "price-current", EffectiveAt: now},
			{ID: "price-future", EffectiveAt: now.Add(time.Hour)},
		},
	}
	if err := source.ApplySnapshot(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if !source.Ready() {
		t.Fatal("source should be ready after snapshot application")
	}
	if status := source.SnapshotStatus(); status.PriceRevisionID != "price-current" || source.PriceRevisionID() != "price-current" {
		t.Fatalf("active price revision was not applied atomically: %#v", status)
	}
	candidates, err := source.Candidates(context.Background(), "code-fast")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Provider != "mock" || candidates[0].ConcurrencyLimit != 4 {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
	models := source.Models()
	if len(models) != 1 || models[0] != "code-fast" {
		t.Fatalf("signed snapshot retained an implicit route: %#v", models)
	}

	source.now = func() time.Time { return now.Add(2 * time.Hour) }
	if source.Ready() {
		t.Fatal("source should fail readiness after snapshot expiry")
	}
}

func TestCandidateSourceRejectsReusedAndOlderSnapshotRevisions(t *testing.T) {
	t.Parallel()
	definitions := map[string]providerDefinition{
		"mock": {Adapter: mock.New("ok"), Protocol: canonical.ProtocolOpenAIResponses, Configured: true},
	}
	source, err := newCandidateSource(definitions, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	source.now = func() time.Time { return now }
	makeSnapshot := func(revision int64, model string) snapshot.Snapshot {
		return snapshot.Snapshot{
			Revision:    revision,
			GeneratedAt: now,
			ExpiresAt:   now.Add(time.Hour),
			Models: []snapshot.ModelRoute{{
				VirtualModel: "code-fast",
				CandidateDetails: []snapshot.RouteCandidate{{
					Provider: "mock", Model: model, Protocol: string(canonical.ProtocolOpenAIResponses),
				}},
			}},
		}
	}
	if err := source.ApplySnapshot(context.Background(), makeSnapshot(12, "local/echo")); err != nil {
		t.Fatal(err)
	}
	if err := source.ApplySnapshot(context.Background(), makeSnapshot(12, "different")); err == nil {
		t.Fatal("expected reused revision to be rejected")
	}
	if err := source.ApplySnapshot(context.Background(), makeSnapshot(11, "older")); err == nil {
		t.Fatal("expected rollback revision to be rejected")
	}
	candidates, err := source.Candidates(context.Background(), "code-fast")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Model != "local/echo" {
		t.Fatalf("rejected snapshot changed live routes: %#v", candidates)
	}
	if status := source.SnapshotStatus(); status.Revision != 12 {
		t.Fatalf("revision changed after rejected snapshot: %#v", status)
	}
}

func TestCandidateSourceKeepsLocalEchoOnlyWhenSnapshotDeclaresIt(t *testing.T) {
	t.Parallel()
	definitions := map[string]providerDefinition{
		"mock": {Adapter: mock.New("ok"), Protocol: canonical.ProtocolOpenAIResponses, Configured: true},
	}
	source, err := newCandidateSource(definitions, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	source.now = func() time.Time { return now }
	value := snapshot.Snapshot{
		Revision: 1, GeneratedAt: now, ExpiresAt: now.Add(time.Hour),
		Models: []snapshot.ModelRoute{{
			VirtualModel: "local/echo",
			CandidateDetails: []snapshot.RouteCandidate{{
				Provider: "mock", Model: "local/echo", Protocol: string(canonical.ProtocolOpenAIResponses),
			}},
		}},
	}
	if err := source.ApplySnapshot(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Candidates(context.Background(), "local/echo"); err != nil {
		t.Fatalf("explicitly signed local route was removed: %v", err)
	}
}
