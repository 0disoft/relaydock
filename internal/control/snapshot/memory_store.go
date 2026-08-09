package snapshot

import (
	"context"
	"sync"

	"github.com/0disoft/relaydock/internal/core"
)

type MemoryStore struct {
	mu       sync.RWMutex
	current  Snapshot
	set      bool
	watchers map[int]chan Snapshot
	next     int
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{watchers: map[int]chan Snapshot{}} }
func (s *MemoryStore) Current(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.set {
		return Snapshot{}, core.ErrNotFound
	}
	return cloneSnapshot(s.current), nil
}
func (s *MemoryStore) Publish(ctx context.Context, v Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.set && v.Revision <= s.current.Revision {
		s.mu.Unlock()
		return core.ErrConflict
	}
	s.current = cloneSnapshot(v)
	s.set = true
	watchers := make([]chan Snapshot, 0, len(s.watchers))
	for _, ch := range s.watchers {
		watchers = append(watchers, ch)
	}
	s.mu.Unlock()
	for _, ch := range watchers {
		select {
		case ch <- cloneSnapshot(v):
		default:
		}
	}
	return nil
}
func (s *MemoryStore) Watch(ctx context.Context, after int64) (<-chan Snapshot, error) {
	ch := make(chan Snapshot, 1)
	s.mu.Lock()
	id := s.next
	s.next++
	s.watchers[id] = ch
	if s.set && s.current.Revision > after {
		ch <- cloneSnapshot(s.current)
	}
	s.mu.Unlock()
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		if existing, ok := s.watchers[id]; ok {
			delete(s.watchers, id)
			close(existing)
		}
		s.mu.Unlock()
	}()
	return ch, nil
}
func cloneSnapshot(v Snapshot) Snapshot {
	v.VirtualKeys = append([]VirtualKey(nil), v.VirtualKeys...)
	for index := range v.VirtualKeys {
		v.VirtualKeys[index].AllowedModels = append([]string(nil), v.VirtualKeys[index].AllowedModels...)
		v.VirtualKeys[index].Scopes = append([]string(nil), v.VirtualKeys[index].Scopes...)
	}
	v.Models = append([]ModelRoute(nil), v.Models...)
	for index := range v.Models {
		v.Models[index].Candidates = append([]string(nil), v.Models[index].Candidates...)
		v.Models[index].CandidateDetails = append([]RouteCandidate(nil), v.Models[index].CandidateDetails...)
	}
	v.ProviderRefs = append([]ProviderReference(nil), v.ProviderRefs...)
	v.PriceRevisions = append([]PriceRevision(nil), v.PriceRevisions...)
	v.Signature = append([]byte(nil), v.Signature...)
	return v
}
