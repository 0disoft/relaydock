package localstore

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/persistence/atomicfile"
)

func Open(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%w: local expert store path", core.ErrInvalidArgument)
	}
	store := &Store{
		path:   path,
		now:    func() time.Time { return time.Now().UTC() },
		chunks: newChunkStore(path, defaultContentChunk),
		data:   emptyState(),
	}
	raw, err := atomicfile.Read(path, maximumStateBytes)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return store, nil
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, fmt.Errorf("decode local expert store header: %w", err)
	}
	switch header.Version {
	case 0, 1, 2:
		migrated, err := store.migrateLegacy(raw)
		if err != nil {
			return nil, err
		}
		store.data = migrated
		if err := atomicfile.Write(path+".v2.bak", raw, 0o600); err != nil {
			return nil, fmt.Errorf("backup legacy local expert store: %w", err)
		}
		if err := store.persistLocked(migrated); err != nil {
			return nil, fmt.Errorf("persist migrated local expert store: %w", err)
		}
	case currentVersion:
		if err := json.Unmarshal(raw, &store.data); err != nil {
			return nil, fmt.Errorf("decode local expert store: %w", err)
		}
		if err := normalizeAndValidate(&store.data); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: unsupported local expert store version %d", core.ErrInvalidConfiguration, header.Version)
	}
	if err := store.verifyChunkReferences(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) migrateLegacy(raw []byte) (state, error) {
	var legacy legacyState
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return state{}, fmt.Errorf("decode legacy local expert store: %w", err)
	}
	if err := normalizeLegacy(&legacy); err != nil {
		return state{}, err
	}
	next := state{
		Version:        currentVersion,
		Consultations:  legacy.Consultations,
		Idempotency:    legacy.Idempotency,
		ContextPacks:   make(map[string]storedPack, len(legacy.ContextPacks)),
		Results:        legacy.Results,
		ResultMetadata: legacy.ResultMetadata,
	}
	for id, pack := range legacy.ContextPacks {
		stored, err := s.prepareStoredPack(pack)
		if err != nil {
			return state{}, fmt.Errorf("migrate context pack %s: %w", id, err)
		}
		next.ContextPacks[id] = stored
	}
	if err := normalizeAndValidate(&next); err != nil {
		return state{}, err
	}
	return next, nil
}
