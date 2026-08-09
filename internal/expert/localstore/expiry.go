package localstore

import (
	"context"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
)

func (s *Store) expireIfNeeded(ctx context.Context, id string) error {
	s.mu.RLock()
	item, exists := s.data.Consultations[id]
	needsExpiration := exists && !item.Terminal() && !item.ExpiresAt.IsZero() && !item.ExpiresAt.After(s.nowUTC())
	s.mu.RUnlock()
	if !needsExpiration {
		return nil
	}
	return s.update(ctx, func(next *state) error {
		item, exists := next.Consultations[id]
		if !exists {
			return nil
		}
		expireConsultation(&item, s.nowUTC())
		next.Consultations[id] = item
		return nil
	})
}

func (s *Store) expireAll(ctx context.Context) error {
	s.mu.RLock()
	now := s.nowUTC()
	needsExpiration := false
	for _, item := range s.data.Consultations {
		if !item.Terminal() && !item.ExpiresAt.IsZero() && !item.ExpiresAt.After(now) {
			needsExpiration = true
			break
		}
	}
	s.mu.RUnlock()
	if !needsExpiration {
		return nil
	}
	return s.update(ctx, func(next *state) error {
		for id, item := range next.Consultations {
			if expireConsultation(&item, now) {
				next.Consultations[id] = item
			}
		}
		return nil
	})
}

func expireConsultation(item *consultation.Consultation, now time.Time) bool {
	if item == nil || item.Terminal() || item.ExpiresAt.IsZero() || item.ExpiresAt.After(now) {
		return false
	}
	item.State = consultation.StateExpired
	item.UpdatedAt = now
	consultation.ClearClaim(item)
	return true
}
