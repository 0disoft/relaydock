package localstore

import (
	"context"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
)

// CreateWithContext stores a ContextPack and its first consultation in one
// metadata transaction. Immutable chunks are written first; a crash before the
// metadata commit can only leave unreferenced chunks, which Compact reclaims.
func (s *Store) CreateWithContext(ctx context.Context, pack contextpack.Pack, command consultation.CreateCommand) (consultation.Consultation, contextpack.Pack, error) {
	s.chunkMu.RLock()
	defer s.chunkMu.RUnlock()
	stored, err := s.prepareStoredPack(pack)
	if err != nil {
		return consultation.Consultation{}, contextpack.Pack{}, err
	}
	packID := stored.Manifest.ID
	command.ContextPackID = packID
	var created consultation.Consultation
	var selected storedPack
	err = s.update(ctx, func(next *state) error {
		scope := consultation.IdempotencyScope(command)
		fingerprint := consultation.RequestFingerprint(command)
		if scope != "" {
			if record, exists := next.Idempotency[scope]; exists {
				if record.Fingerprint != fingerprint {
					return core.ErrConflict
				}
				item, exists := next.Consultations[record.ConsultationID]
				if !exists {
					return core.ErrInvalidConfiguration
				}
				existing, exists := next.ContextPacks[item.ContextPackID]
				if !exists {
					return core.ErrInvalidConfiguration
				}
				created, selected = item, existing
				return nil
			}
		}
		if existing, exists := next.ContextPacks[packID]; exists {
			equal, err := equalStoredPacks(existing, stored)
			if err != nil {
				return err
			}
			if !equal {
				return core.ErrConflict
			}
			selected = existing
		} else {
			next.ContextPacks[packID] = stored
			selected = stored
		}
		item, _, err := s.createInState(next, command)
		if err != nil {
			return err
		}
		created = item
		return nil
	})
	if err != nil {
		return consultation.Consultation{}, contextpack.Pack{}, err
	}
	hydrated, err := s.hydrateStoredPack(selected)
	if err != nil {
		return consultation.Consultation{}, contextpack.Pack{}, err
	}
	return cloneConsultation(created), hydrated, nil
}
