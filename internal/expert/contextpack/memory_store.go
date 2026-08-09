package contextpack

import (
	"context"
	"sync"

	"github.com/0disoft/relaydock/internal/core"
)

type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]Pack
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: make(map[string]Pack)}
}

func (s *MemoryStore) Put(ctx context.Context, pack Pack) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if pack.ID == "" {
		return core.ErrInvalidArgument
	}
	s.mu.Lock()
	s.items[pack.ID] = clonePack(pack)
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, id string) (Pack, error) {
	if err := ctx.Err(); err != nil {
		return Pack{}, err
	}
	s.mu.RLock()
	pack, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		return Pack{}, core.ErrNotFound
	}
	return clonePack(pack), nil
}

func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return core.ErrNotFound
	}
	delete(s.items, id)
	return nil
}

func clonePack(pack Pack) Pack {
	pack.SuccessCriteria = append([]string(nil), pack.SuccessCriteria...)
	pack.Attempts = append([]Attempt(nil), pack.Attempts...)
	pack.OpenQuestions = append([]string(nil), pack.OpenQuestions...)
	pack.Evidence = append([]Evidence(nil), pack.Evidence...)
	pack.RedactionReport.Findings = append([]RedactionFinding(nil), pack.RedactionReport.Findings...)
	return pack
}
