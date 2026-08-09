package autostart

import (
	"context"
	"sync"
)

// MemoryService provides deterministic behaviour for tests and headless mode.
// Platform adapters may wrap launchd, systemd user units, or Windows startup.
type MemoryService struct {
	mu      sync.RWMutex
	enabled bool
}

func (s *MemoryService) Enable(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.enabled = true
	s.mu.Unlock()
	return nil
}

func (s *MemoryService) Disable(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.enabled = false
	s.mu.Unlock()
	return nil
}

func (s *MemoryService) Enabled(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled, nil
}
