package credentials

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/0disoft/relaydock/internal/core"
)

// MemoryStore is intended for tests, local ephemeral sessions, and dependency
// injection. Values are copied on both write and read so callers cannot mutate
// stored credentials through shared slices.
type MemoryStore struct {
	mu     sync.RWMutex
	values map[string][]byte
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{values: make(map[string][]byte)}
}

func (s *MemoryStore) Put(ctx context.Context, ref Reference, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key, err := credentialKey(ref)
	if err != nil {
		return err
	}
	if len(value) == 0 {
		return fmt.Errorf("%w: empty credential", core.ErrInvalidArgument)
	}
	s.mu.Lock()
	s.values[key] = append([]byte(nil), value...)
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, ref Reference) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, err := credentialKey(ref)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	value, ok := s.values[key]
	s.mu.RUnlock()
	if !ok {
		return nil, core.ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *MemoryStore) Delete(ctx context.Context, ref Reference) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key, err := credentialKey(ref)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if value, ok := s.values[key]; ok {
		for i := range value {
			value[i] = 0
		}
		delete(s.values, key)
	}
	s.mu.Unlock()
	return nil
}

func credentialKey(ref Reference) (string, error) {
	id := strings.TrimSpace(ref.ID)
	provider := strings.ToLower(strings.TrimSpace(ref.Provider))
	account := strings.TrimSpace(ref.Account)
	if id == "" || provider == "" {
		return "", fmt.Errorf("%w: credential id and provider are required", core.ErrInvalidArgument)
	}
	return provider + "\x00" + account + "\x00" + id, nil
}
