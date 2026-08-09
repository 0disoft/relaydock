package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/persistence/atomicfile"
)

// LocalStore persists the latest signed snapshot for a single controld
// instance. It provides the same in-process watch semantics as MemoryStore and
// writes the replacement file before notifying watchers.
type LocalStore struct {
	mu       sync.RWMutex
	path     string
	current  Snapshot
	set      bool
	watchers map[int]chan Snapshot
	next     int
}

func OpenLocalStore(path string) (*LocalStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%w: control snapshot path", core.ErrInvalidArgument)
	}
	store := &LocalStore{path: path, watchers: make(map[int]chan Snapshot)}
	raw, err := atomicfile.Read(path, 32<<20)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return store, nil
	}
	if err := json.Unmarshal(raw, &store.current); err != nil {
		return nil, fmt.Errorf("decode control snapshot: %w", err)
	}
	if store.current.Revision <= 0 || store.current.GeneratedAt.IsZero() || store.current.ExpiresAt.IsZero() {
		return nil, fmt.Errorf("%w: invalid persisted control snapshot", core.ErrInvalidConfiguration)
	}
	store.current = cloneSnapshot(store.current)
	store.set = true
	return store, nil
}

func (s *LocalStore) Current(ctx context.Context) (Snapshot, error) {
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

func (s *LocalStore) Publish(ctx context.Context, value Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.set && value.Revision <= s.current.Revision {
		s.mu.Unlock()
		return core.ErrConflict
	}
	persisted := cloneSnapshot(value)
	raw, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("encode control snapshot: %w", err)
	}
	raw = append(raw, '\n')
	if err := atomicfile.Write(s.path, raw, 0o600); err != nil {
		s.mu.Unlock()
		return err
	}
	s.current = persisted
	s.set = true
	watchers := make([]chan Snapshot, 0, len(s.watchers))
	for _, watcher := range s.watchers {
		watchers = append(watchers, watcher)
	}
	s.mu.Unlock()
	for _, watcher := range watchers {
		select {
		case watcher <- cloneSnapshot(persisted):
		default:
		}
	}
	return nil
}

func (s *LocalStore) Watch(ctx context.Context, after int64) (<-chan Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	watcher := make(chan Snapshot, 1)
	s.mu.Lock()
	id := s.next
	s.next++
	s.watchers[id] = watcher
	if s.set && s.current.Revision > after {
		watcher <- cloneSnapshot(s.current)
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
	return watcher, nil
}
