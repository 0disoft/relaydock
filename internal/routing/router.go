package routing

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

type Router interface {
	Select(context.Context, Request, []Candidate) (Decision, error)
}
type Service struct {
	scorer      Scorer
	decisionTTL time.Duration
}

func New(scorer Scorer) *Service {
	if scorer == nil {
		scorer = DefaultScorer()
	}
	return &Service{scorer: scorer, decisionTTL: 15 * time.Second}
}
func (s *Service) Select(ctx context.Context, req Request, candidates []Candidate) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	eligible := make([]Decision, 0, len(candidates))
	for _, c := range candidates {
		if !c.Available {
			continue
		}
		if !c.Capabilities.Supports(req.Requirements) {
			continue
		}
		if !req.Requirements.AllowsRegion(c.Region) {
			continue
		}
		if len(req.AllowedProviders) > 0 && !containsFold(req.AllowedProviders, c.Provider) {
			continue
		}
		if len(req.AllowedRegions) > 0 && !containsFold(req.AllowedRegions, c.Region) {
			continue
		}
		if req.MaximumCost > 0 && c.EstimatedCost > req.MaximumCost {
			continue
		}
		reasons := []string{fmt.Sprintf("health=%.2f", c.HealthScore), fmt.Sprintf("queue=%d", c.QueueDepth)}
		if c.SessionAffinity {
			reasons = append(reasons, "session_affinity")
		}
		if c.CacheAffinity {
			reasons = append(reasons, "cache_affinity")
		}
		eligible = append(eligible, Decision{Candidate: c, Score: s.scorer.Score(req, c), ExpiresAt: time.Now().UTC().Add(s.decisionTTL), Reasons: reasons})
	}
	if len(eligible) == 0 {
		return Decision{}, core.ErrNoRoute
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		if eligible[i].Score != eligible[j].Score {
			return eligible[i].Score > eligible[j].Score
		}
		a, b := eligible[i].Candidate, eligible[j].Candidate
		return a.Provider+"/"+a.AccountID+"/"+a.Model < b.Provider+"/"+b.AccountID+"/"+b.Model
	})
	return eligible[0], nil
}
func containsFold(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}
