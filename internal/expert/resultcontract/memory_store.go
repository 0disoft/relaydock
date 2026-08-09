package resultcontract

import (
	"context"
	"sync"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
)

type Store interface {
	Put(context.Context, Result) (string, error)
	Get(context.Context, string) (Result, error)
}

type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]Record
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{items: map[string]Record{}} }

func (s *MemoryStore) Put(ctx context.Context, result Result) (string, error) {
	return s.PutRecord(ctx, Record{Result: result, ModelAttestation: UnverifiedAttestation})
}

func (s *MemoryStore) PutRecord(ctx context.Context, record Record) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	record = NormalizeRecord(record)
	if err := Validate(record.Result); err != nil {
		return "", err
	}
	id := idgen.New("result")
	s.mu.Lock()
	s.items[id] = cloneRecord(record)
	s.mu.Unlock()
	return id, nil
}

func (s *MemoryStore) Get(ctx context.Context, id string) (Result, error) {
	record, err := s.GetRecord(ctx, id)
	return record.Result, err
}

func (s *MemoryStore) GetRecord(ctx context.Context, id string) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	s.mu.RLock()
	record, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		return Record{}, core.ErrNotFound
	}
	return cloneRecord(record), nil
}

func cloneRecord(record Record) Record {
	record.Result = cloneResultValue(record.Result)
	return record
}
