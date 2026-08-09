package localstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/expert/contextpack"
)

func (s *Store) Put(ctx context.Context, pack contextpack.Pack) error {
	s.chunkMu.RLock()
	defer s.chunkMu.RUnlock()
	pack.ID = strings.TrimSpace(pack.ID)
	if pack.ID == "" {
		return core.ErrInvalidArgument
	}
	stored, err := s.prepareStoredPack(pack)
	if err != nil {
		return err
	}
	return s.update(ctx, func(next *state) error {
		if existing, exists := next.ContextPacks[pack.ID]; exists {
			equal, err := equalStoredPacks(existing, stored)
			if err != nil {
				return err
			}
			if !equal {
				return fmt.Errorf("%w: context pack ID already contains different data", core.ErrConflict)
			}
			return nil
		}
		next.ContextPacks[pack.ID] = stored
		return nil
	})
}

func (s *Store) GetPack(ctx context.Context, id string) (contextpack.Pack, error) {
	s.chunkMu.RLock()
	defer s.chunkMu.RUnlock()
	if err := ctx.Err(); err != nil {
		return contextpack.Pack{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return contextpack.Pack{}, core.ErrInvalidArgument
	}
	s.mu.RLock()
	stored, exists := s.data.ContextPacks[id]
	stored = cloneStoredPack(stored)
	s.mu.RUnlock()
	if !exists {
		return contextpack.Pack{}, core.ErrNotFound
	}
	return s.hydrateStoredPack(stored)
}

func (s *Store) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return core.ErrInvalidArgument
	}
	return s.update(ctx, func(next *state) error {
		if _, exists := next.ContextPacks[id]; !exists {
			return core.ErrNotFound
		}
		for _, item := range next.Consultations {
			if item.ContextPackID == id {
				return core.ErrConflict
			}
		}
		delete(next.ContextPacks, id)
		return nil
	})
}

func (s *Store) prepareStoredPack(pack contextpack.Pack) (storedPack, error) {
	pack = clonePack(pack)
	pack.ID = strings.TrimSpace(pack.ID)
	if pack.ID == "" {
		return storedPack{}, core.ErrInvalidArgument
	}
	stored := storedPack{Manifest: pack}
	for index := range stored.Manifest.Evidence {
		evidence := &stored.Manifest.Evidence[index]
		content := []byte(evidence.Content)
		if len(content) == 0 {
			continue
		}
		digest := contextpack.Digest(content)
		if strings.TrimSpace(evidence.Digest) != "" && evidence.Digest != digest {
			return storedPack{}, fmt.Errorf("%w: evidence %s digest mismatch", core.ErrInvalidArgument, evidence.Reference)
		}
		evidence.Digest = digest
		evidence.Bytes = int64(len(content))
		references, err := s.chunks.put(content)
		if err != nil {
			return storedPack{}, err
		}
		stored.Contents = append(stored.Contents, evidenceContent{
			EvidenceIndex: index, Digest: digest, Bytes: int64(len(content)), Chunks: references,
		})
		evidence.Content = ""
	}
	return stored, nil
}

func (s *Store) hydrateStoredPack(stored storedPack) (contextpack.Pack, error) {
	pack := clonePack(stored.Manifest)
	for _, content := range stored.Contents {
		if content.EvidenceIndex < 0 || content.EvidenceIndex >= len(pack.Evidence) {
			return contextpack.Pack{}, fmt.Errorf("%w: invalid evidence content index", core.ErrInvalidConfiguration)
		}
		raw, err := s.chunks.get(content.Chunks, content.Bytes, content.Digest)
		if err != nil {
			return contextpack.Pack{}, err
		}
		pack.Evidence[content.EvidenceIndex].Content = string(raw)
	}
	return pack, nil
}

func (s *Store) verifyChunkReferences() error {
	s.chunkMu.RLock()
	defer s.chunkMu.RUnlock()
	s.mu.RLock()
	defer s.mu.RUnlock()
	for packID, stored := range s.data.ContextPacks {
		for _, content := range stored.Contents {
			if err := s.chunks.verifyReferences(content.Chunks); err != nil {
				return fmt.Errorf("verify context pack %s: %w", packID, err)
			}
		}
	}
	return nil
}

func equalStoredPacks(left, right storedPack) (bool, error) {
	leftRaw, err := json.Marshal(left)
	if err != nil {
		return false, err
	}
	rightRaw, err := json.Marshal(right)
	if err != nil {
		return false, err
	}
	return string(leftRaw) == string(rightRaw), nil
}
