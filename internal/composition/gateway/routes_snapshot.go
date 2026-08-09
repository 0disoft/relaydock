package gateway

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/control/snapshot"
	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
)

func (s *CandidateSource) Models() []string {
	s.routeMu.RLock()
	models := make([]string, 0, len(s.routes))
	for model := range s.routes {
		models = append(models, model)
	}
	s.routeMu.RUnlock()
	sort.Strings(models)
	return models
}

func (s *CandidateSource) SetRequireFreshSnapshot(required bool) {
	s.mu.Lock()
	s.requireFreshSnapshot = required
	s.mu.Unlock()
}

// SetAllowDirectRouting controls provider/model and default-provider fallback.
// Signed control deployments should leave this disabled so every public model
// must be named in the signed route table instead of bypassing policy through a
// raw upstream model identifier.
func (s *CandidateSource) SetAllowDirectRouting(allowed bool) {
	s.mu.Lock()
	s.allowDirectRouting = allowed
	s.mu.Unlock()
}

func (s *CandidateSource) DirectRoutingAllowed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allowDirectRouting
}

func (s *CandidateSource) Ready() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.requireFreshSnapshot {
		return true
	}
	return s.snapshotRevision > 0 && s.snapshotExpiresAt.After(s.currentTimeLocked())
}

func (s *CandidateSource) SnapshotStatus() SnapshotStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	ready := !s.requireFreshSnapshot || (s.snapshotRevision > 0 && s.snapshotExpiresAt.After(s.currentTimeLocked()))
	return SnapshotStatus{
		Required:           s.requireFreshSnapshot,
		AllowDirectRouting: s.allowDirectRouting,
		Revision:           s.snapshotRevision,
		ExpiresAt:          s.snapshotExpiresAt,
		PriceRevisionID:    s.snapshotPriceRevisionID,
		Ready:              ready,
	}
}

func (s *CandidateSource) ApplySnapshot(ctx context.Context, value snapshot.Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := snapshot.Validate(value, snapshot.ValidationOptions{Now: s.currentTime(), RequireUnexpired: true}); err != nil {
		return err
	}
	routes, err := routeConfigFromSnapshot(value)
	if err != nil {
		return err
	}
	priceRevisionID := activePriceRevisionID(value.PriceRevisions, s.currentTime())
	replacement, err := newCandidateSource(s.providers, routes, s.defaultProvider)
	if err != nil {
		return err
	}
	// A signed snapshot is authoritative. Development-only implicit routes such
	// as local/echo must not survive unless the signed document explicitly
	// declares them. Otherwise an operator could enable the mock adapter and
	// accidentally create a route outside the signed policy surface.
	if _, explicitlyConfigured := routes["local/echo"]; !explicitlyConfigured {
		delete(replacement.routes, "local/echo")
	}
	// Revision validation and route replacement form one atomic critical section.
	// Without this lock ordering, concurrent revision N and N+1 applications
	// could leave the newer revision metadata paired with the older route map.
	s.routeMu.Lock()
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.routeMu.Unlock()
	if value.Revision <= s.snapshotRevision {
		return fmt.Errorf("%w: runtime route snapshot revision %d is not newer than %d", core.ErrConflict, value.Revision, s.snapshotRevision)
	}
	s.routes = replacement.routes
	s.snapshotRevision = value.Revision
	s.snapshotExpiresAt = value.ExpiresAt.UTC()
	s.snapshotPriceRevisionID = priceRevisionID
	return nil
}

// PriceRevisionID returns the pricing revision atomically associated with the
// currently active signed route snapshot. Static-only deployments return an
// empty string and allow the processor's configured fallback to remain in use.
func (s *CandidateSource) PriceRevisionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotPriceRevisionID
}

func activePriceRevisionID(revisions []snapshot.PriceRevision, now time.Time) string {
	var selected snapshot.PriceRevision
	for _, revision := range revisions {
		effectiveAt := revision.EffectiveAt.UTC()
		if effectiveAt.After(now) {
			continue
		}
		if selected.ID == "" || effectiveAt.After(selected.EffectiveAt) || (effectiveAt.Equal(selected.EffectiveAt) && revision.ID > selected.ID) {
			selected = revision
			selected.EffectiveAt = effectiveAt
		}
	}
	return strings.TrimSpace(selected.ID)
}

func routeConfigFromSnapshot(value snapshot.Snapshot) (map[string][]RouteConfig, error) {
	routes := make(map[string][]RouteConfig, len(value.Models))
	for _, modelRoute := range value.Models {
		virtualModel := strings.TrimSpace(modelRoute.VirtualModel)
		configured := make([]RouteConfig, 0, len(modelRoute.CandidateDetails)+len(modelRoute.Candidates))
		if len(modelRoute.CandidateDetails) > 0 {
			for _, candidate := range modelRoute.CandidateDetails {
				configured = append(configured, RouteConfig{
					Provider:         candidate.Provider,
					AccountID:        candidate.AccountID,
					Model:            candidate.Model,
					Protocol:         canonical.Protocol(candidate.Protocol),
					Region:           candidate.Region,
					EstimatedCost:    candidate.EstimatedCost,
					HealthScore:      candidate.HealthScore,
					QueueDepth:       candidate.QueueDepth,
					ConcurrencyLimit: candidate.ConcurrencyLimit,
					Disabled:         candidate.Disabled,
				})
			}
		} else {
			for _, compact := range modelRoute.Candidates {
				providerName, upstreamModel, err := parseCompactCandidate(compact)
				if err != nil {
					return nil, fmt.Errorf("route %s: %w", virtualModel, err)
				}
				configured = append(configured, RouteConfig{Provider: providerName, Model: upstreamModel})
			}
		}
		routes[virtualModel] = configured
	}
	return routes, nil
}

func parseCompactCandidate(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "local/echo" {
		return "mock", "local/echo", nil
	}
	providerName, model, found := strings.Cut(value, "/")
	if !found || strings.TrimSpace(providerName) == "" || strings.TrimSpace(model) == "" {
		return "", "", fmt.Errorf("%w: compact candidate %q must be provider/model", core.ErrInvalidConfiguration, value)
	}
	return strings.TrimSpace(providerName), strings.TrimSpace(model), nil
}
