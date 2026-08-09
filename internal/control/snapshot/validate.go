package snapshot

import (
	"fmt"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

type ValidationOptions struct {
	Now               time.Time
	MaximumFutureSkew time.Duration
	RequireUnexpired  bool
}

func Validate(value Snapshot, options ValidationOptions) error {
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	maximumFutureSkew := options.MaximumFutureSkew
	if maximumFutureSkew <= 0 {
		maximumFutureSkew = 5 * time.Minute
	}
	if value.Revision <= 0 {
		return fmt.Errorf("%w: snapshot revision must be positive", core.ErrInvalidConfiguration)
	}
	if value.GeneratedAt.IsZero() || value.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: snapshot timestamps are required", core.ErrInvalidConfiguration)
	}
	if !value.ExpiresAt.After(value.GeneratedAt) {
		return fmt.Errorf("%w: snapshot expiry must follow generation", core.ErrInvalidConfiguration)
	}
	if value.GeneratedAt.After(now.Add(maximumFutureSkew)) {
		return fmt.Errorf("%w: snapshot generatedAt is too far in the future", core.ErrInvalidConfiguration)
	}
	if options.RequireUnexpired && !value.ExpiresAt.After(now) {
		return fmt.Errorf("%w: runtime snapshot expired at %s", core.ErrExpired, value.ExpiresAt.UTC().Format(time.RFC3339))
	}
	if len(value.Models) == 0 {
		return fmt.Errorf("%w: runtime snapshot has no model routes", core.ErrInvalidConfiguration)
	}
	seen := make(map[string]struct{}, len(value.Models))
	for _, route := range value.Models {
		name := strings.TrimSpace(route.VirtualModel)
		if name == "" {
			return fmt.Errorf("%w: snapshot route has an empty virtual model", core.ErrInvalidConfiguration)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("%w: duplicate snapshot route %q", core.ErrConflict, name)
		}
		seen[name] = struct{}{}
		if len(route.Candidates) == 0 && len(route.CandidateDetails) == 0 {
			return fmt.Errorf("%w: snapshot route %q has no candidates", core.ErrInvalidConfiguration, name)
		}
		for _, compact := range route.Candidates {
			if strings.TrimSpace(compact) == "" {
				return fmt.Errorf("%w: snapshot route %q contains an empty candidate", core.ErrInvalidConfiguration, name)
			}
		}
		for _, candidate := range route.CandidateDetails {
			if strings.TrimSpace(candidate.Provider) == "" || strings.TrimSpace(candidate.Model) == "" {
				return fmt.Errorf("%w: snapshot route %q candidate requires provider and model", core.ErrInvalidConfiguration, name)
			}
			if candidate.EstimatedCost < 0 || candidate.QueueDepth < 0 || candidate.ConcurrencyLimit < 0 || candidate.HealthScore < 0 {
				return fmt.Errorf("%w: snapshot route %q candidate contains negative values", core.ErrInvalidConfiguration, name)
			}
		}
	}
	seenPrices := make(map[string]struct{}, len(value.PriceRevisions))
	activePriceRevision := len(value.PriceRevisions) == 0
	for _, revision := range value.PriceRevisions {
		id := strings.TrimSpace(revision.ID)
		if id == "" || revision.EffectiveAt.IsZero() {
			return fmt.Errorf("%w: price revision requires id and effectiveAt", core.ErrInvalidConfiguration)
		}
		if _, exists := seenPrices[id]; exists {
			return fmt.Errorf("%w: duplicate price revision %q", core.ErrConflict, id)
		}
		seenPrices[id] = struct{}{}
		if !revision.EffectiveAt.UTC().After(now) {
			activePriceRevision = true
		}
	}
	if !activePriceRevision {
		return fmt.Errorf("%w: snapshot has no currently effective price revision", core.ErrInvalidConfiguration)
	}
	return nil
}
