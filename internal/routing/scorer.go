package routing

import "math"

type Scorer interface {
	Score(Request, Candidate) float64
}
type WeightedScorer struct{ HealthWeight, CostWeight, QueueWeight, AffinityWeight float64 }

func DefaultScorer() WeightedScorer {
	return WeightedScorer{HealthWeight: 0.55, CostWeight: 0.2, QueueWeight: 0.15, AffinityWeight: 0.1}
}
func (s WeightedScorer) Score(req Request, c Candidate) float64 {
	if s.HealthWeight == 0 && s.CostWeight == 0 && s.QueueWeight == 0 && s.AffinityWeight == 0 {
		s = DefaultScorer()
	}
	health := math.Max(0, math.Min(1, c.HealthScore))
	cost := 1.0
	if req.MaximumCost > 0 {
		cost = 1 - math.Min(1, float64(c.EstimatedCost)/float64(req.MaximumCost))
	} else if c.EstimatedCost > 0 {
		cost = 1 / (1 + float64(c.EstimatedCost))
	}
	queue := 1 / (1 + math.Max(0, float64(c.QueueDepth)))
	affinity := 0.0
	if c.SessionAffinity {
		affinity += 0.65
	}
	if c.CacheAffinity {
		affinity += 0.35
	}
	return health*s.HealthWeight + cost*s.CostWeight + queue*s.QueueWeight + affinity*s.AffinityWeight
}
