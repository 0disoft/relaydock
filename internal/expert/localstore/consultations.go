package localstore

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
)

func (s *Store) Create(ctx context.Context, command consultation.CreateCommand) (consultation.Consultation, error) {
	if err := ctx.Err(); err != nil {
		return consultation.Consultation{}, err
	}
	var created consultation.Consultation
	err := s.update(ctx, func(next *state) error {
		item, _, err := s.createInState(next, command)
		if err != nil {
			return err
		}
		created = item
		return nil
	})
	return cloneConsultation(created), err
}

func (s *Store) createInState(next *state, command consultation.CreateCommand) (consultation.Consultation, bool, error) {
	scope := consultation.IdempotencyScope(command)
	fingerprint := consultation.RequestFingerprint(command)
	if scope != "" {
		if record, exists := next.Idempotency[scope]; exists {
			if record.Fingerprint != fingerprint {
				return consultation.Consultation{}, false, core.ErrConflict
			}
			item, exists := next.Consultations[record.ConsultationID]
			if !exists {
				return consultation.Consultation{}, false, core.ErrInvalidConfiguration
			}
			return cloneConsultation(item), false, nil
		}
	}
	now := s.nowUTC()
	ttl := command.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	stateValue := consultation.StateContextPending
	if strings.TrimSpace(command.ContextPackID) != "" {
		stateValue = consultation.StateApprovalPending
	}
	created := consultation.Consultation{
		ID: idgen.New("con"), TenantID: strings.TrimSpace(command.TenantID), ProjectID: strings.TrimSpace(command.ProjectID),
		Objective: strings.TrimSpace(command.Objective), TaskType: strings.TrimSpace(command.TaskType), Route: command.Route,
		State: stateValue, ContextPackID: strings.TrimSpace(command.ContextPackID), MaximumCostMinor: command.MaximumCostMinor,
		CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(ttl),
	}
	if created.ContextPackID != "" {
		if _, exists := next.ContextPacks[created.ContextPackID]; !exists {
			return consultation.Consultation{}, false, core.ErrNotFound
		}
	}
	next.Consultations[created.ID] = created
	if scope != "" {
		next.Idempotency[scope] = idempotencyRecord{ConsultationID: created.ID, Fingerprint: fingerprint}
	}
	return created, true, nil
}

func (s *Store) Get(ctx context.Context, id string) (consultation.Consultation, error) {
	if err := ctx.Err(); err != nil {
		return consultation.Consultation{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return consultation.Consultation{}, core.ErrInvalidArgument
	}
	if err := s.expireIfNeeded(ctx, id); err != nil {
		return consultation.Consultation{}, err
	}
	s.mu.RLock()
	item, exists := s.data.Consultations[id]
	s.mu.RUnlock()
	if !exists {
		return consultation.Consultation{}, core.ErrNotFound
	}
	return cloneConsultation(item), nil
}

func (s *Store) List(ctx context.Context, limit int) ([]consultation.Consultation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.expireAll(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	items := make([]consultation.Consultation, 0, len(s.data.Consultations))
	for _, item := range s.data.Consultations {
		items = append(items, cloneConsultation(item))
	}
	s.mu.RUnlock()
	sort.Slice(items, func(left, right int) bool {
		if !items[left].UpdatedAt.Equal(items[right].UpdatedAt) {
			return items[left].UpdatedAt.After(items[right].UpdatedAt)
		}
		return items[left].ID > items[right].ID
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *Store) UpdateState(ctx context.Context, id string, expected, nextState consultation.State) (consultation.Consultation, error) {
	var updated consultation.Consultation
	err := s.update(ctx, func(next *state) error {
		item, exists := next.Consultations[id]
		if !exists {
			return core.ErrNotFound
		}
		if expireConsultation(&item, s.nowUTC()) {
			next.Consultations[id] = item
			return core.ErrConflict
		}
		if item.State != expected {
			return core.ErrConflict
		}
		item.State = nextState
		item.UpdatedAt = s.nowUTC()
		if nextState != consultation.StateRunning {
			consultation.ClearClaim(&item)
		}
		next.Consultations[id] = item
		updated = item
		return nil
	})
	return cloneConsultation(updated), err
}

func (s *Store) AttachContext(ctx context.Context, id, packID string, expected, nextState consultation.State) (consultation.Consultation, error) {
	packID = strings.TrimSpace(packID)
	if packID == "" {
		return consultation.Consultation{}, core.ErrInvalidArgument
	}
	var updated consultation.Consultation
	err := s.update(ctx, func(next *state) error {
		if _, exists := next.ContextPacks[packID]; !exists {
			return core.ErrNotFound
		}
		item, exists := next.Consultations[id]
		if !exists {
			return core.ErrNotFound
		}
		if item.State != expected {
			return core.ErrConflict
		}
		item.ContextPackID = packID
		item.State = nextState
		item.UpdatedAt = s.nowUTC()
		consultation.ClearClaim(&item)
		next.Consultations[id] = item
		updated = item
		return nil
	})
	return cloneConsultation(updated), err
}

func (s *Store) SetFailure(ctx context.Context, id, reason string) (consultation.Consultation, error) {
	var updated consultation.Consultation
	err := s.update(ctx, func(next *state) error {
		item, exists := next.Consultations[id]
		if !exists {
			return core.ErrNotFound
		}
		if item.Terminal() {
			return core.ErrConflict
		}
		item.State = consultation.StateFailed
		item.FailureReason = strings.TrimSpace(reason)
		item.UpdatedAt = s.nowUTC()
		consultation.ClearClaim(&item)
		next.Consultations[id] = item
		updated = item
		return nil
	})
	return cloneConsultation(updated), err
}
