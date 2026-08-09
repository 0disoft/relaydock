package runtime

import (
	"context"
	"github.com/your-org/ai-runtime-gateway/internal/routing"
	"sync"
)

type MemoryCandidateSource struct {
	mu      sync.RWMutex
	byModel map[string][]routing.Candidate
}

func NewMemoryCandidateSource() *MemoryCandidateSource {
	return &MemoryCandidateSource{byModel: map[string][]routing.Candidate{}}
}
func (s *MemoryCandidateSource) Set(model string, candidates []routing.Candidate) {
	s.mu.Lock()
	s.byModel[model] = append([]routing.Candidate(nil), candidates...)
	s.mu.Unlock()
}
func (s *MemoryCandidateSource) Candidates(ctx context.Context, model string) ([]routing.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]routing.Candidate(nil), s.byModel[model]...), nil
}
