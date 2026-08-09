package localstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/0disoft/relaydock/internal/persistence/atomicfile"
)

func (s *Store) update(ctx context.Context, mutate func(*state) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next, err := cloneState(s.data)
	if err != nil {
		return err
	}
	if err := mutate(&next); err != nil {
		return err
	}
	if err := normalizeAndValidate(&next); err != nil {
		return err
	}
	if err := s.persistLocked(next); err != nil {
		return err
	}
	s.data = next
	return nil
}

func (s *Store) persistLocked(next state) error {
	raw, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("encode local expert store: %w", err)
	}
	raw = append(raw, '\n')
	if int64(len(raw)) > maximumStateBytes {
		return fmt.Errorf("local expert metadata exceeds %d bytes", maximumStateBytes)
	}
	return atomicfile.Write(s.path, raw, 0o600)
}

func (s *Store) nowUTC() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}
