package localstore

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/expert/consultation"
)

func (s *Store) ClaimNext(ctx context.Context, route string) (consultation.Consultation, error) {
	return s.Claim(ctx, consultation.ClaimCommand{Route: consultation.Route(route), WorkerID: "legacy", LeaseTTL: 2 * time.Minute, MaxAttempts: 3})
}

func (s *Store) Claim(ctx context.Context, command consultation.ClaimCommand) (consultation.Consultation, error) {
	normalized, err := consultation.NormalizeClaimCommand(command)
	if err != nil {
		return consultation.Consultation{}, err
	}
	var claimed consultation.Consultation
	err = s.update(ctx, func(next *state) error {
		candidateIDs := make([]string, 0)
		for id, item := range next.Consultations {
			if expireConsultation(&item, normalized.ClaimedAt) {
				next.Consultations[id] = item
				continue
			}
			if consultation.Claimable(item, normalized) {
				candidateIDs = append(candidateIDs, id)
			}
		}
		sort.Slice(candidateIDs, func(left, right int) bool {
			a, b := next.Consultations[candidateIDs[left]], next.Consultations[candidateIDs[right]]
			if !a.AvailableAt.Equal(b.AvailableAt) {
				if a.AvailableAt.IsZero() {
					return true
				}
				if b.AvailableAt.IsZero() {
					return false
				}
				return a.AvailableAt.Before(b.AvailableAt)
			}
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.Before(b.CreatedAt)
			}
			return a.ID < b.ID
		})
		for _, id := range candidateIDs {
			item := next.Consultations[id]
			if err := consultation.ApplyClaim(&item, normalized); err != nil {
				next.Consultations[id] = item
				continue
			}
			next.Consultations[id] = item
			claimed = item
			return nil
		}
		return core.ErrNotFound
	})
	return cloneConsultation(claimed), err
}

func (s *Store) RenewClaim(ctx context.Context, id, workerID string, ttl time.Duration) (consultation.Consultation, error) {
	var updated consultation.Consultation
	err := s.update(ctx, func(next *state) error {
		item, exists := next.Consultations[id]
		if !exists {
			return core.ErrNotFound
		}
		if err := consultation.ApplyRenewClaim(&item, workerID, ttl, s.nowUTC()); err != nil {
			return err
		}
		next.Consultations[id] = item
		updated = item
		return nil
	})
	return cloneConsultation(updated), err
}

func (s *Store) RetryClaim(ctx context.Context, id, workerID string, availableAt time.Time, reason string, maximumAttempts int) (consultation.Consultation, error) {
	var updated consultation.Consultation
	err := s.update(ctx, func(next *state) error {
		item, exists := next.Consultations[id]
		if !exists {
			return core.ErrNotFound
		}
		if err := consultation.ApplyRetryClaim(&item, workerID, availableAt, reason, maximumAttempts, s.nowUTC()); err != nil {
			return err
		}
		next.Consultations[id] = item
		updated = item
		return nil
	})
	return cloneConsultation(updated), err
}

func (s *Store) RecoverStaleClaims(ctx context.Context, now time.Time, maximumAttempts int) (int, error) {
	if now.IsZero() {
		now = s.nowUTC()
	}
	recovered := 0
	err := s.update(ctx, func(next *state) error {
		for id, item := range next.Consultations {
			if consultation.ApplyStaleRecovery(&item, now, maximumAttempts) {
				next.Consultations[id] = item
				recovered++
			}
		}
		return nil
	})
	return recovered, err
}

func (s *Store) AttachClaimResult(ctx context.Context, id, workerID, resultID string) (consultation.Consultation, error) {
	resultID = strings.TrimSpace(resultID)
	if resultID == "" {
		return consultation.Consultation{}, core.ErrInvalidArgument
	}
	var updated consultation.Consultation
	err := s.update(ctx, func(next *state) error {
		if _, exists := next.Results[resultID]; !exists {
			return core.ErrNotFound
		}
		item, exists := next.Consultations[id]
		if !exists {
			return core.ErrNotFound
		}
		if err := consultation.ValidateClaimOwner(item, workerID, s.nowUTC()); err != nil {
			return err
		}
		item.ResultID = resultID
		item.State = consultation.StateCompleted
		item.UpdatedAt = s.nowUTC()
		consultation.ClearClaim(&item)
		next.Consultations[id] = item
		updated = item
		return nil
	})
	return cloneConsultation(updated), err
}

func (s *Store) AttachResult(ctx context.Context, id, resultID string) (consultation.Consultation, error) {
	resultID = strings.TrimSpace(resultID)
	if resultID == "" {
		return consultation.Consultation{}, core.ErrInvalidArgument
	}
	var updated consultation.Consultation
	err := s.update(ctx, func(next *state) error {
		if _, exists := next.Results[resultID]; !exists {
			return core.ErrNotFound
		}
		item, exists := next.Consultations[id]
		if !exists {
			return core.ErrNotFound
		}
		if item.State != consultation.StateRunning && item.State != consultation.StateResultPending {
			return core.ErrConflict
		}
		item.ResultID = resultID
		item.State = consultation.StateResultPending
		item.UpdatedAt = s.nowUTC()
		consultation.ClearClaim(&item)
		next.Consultations[id] = item
		updated = item
		return nil
	})
	return cloneConsultation(updated), err
}
