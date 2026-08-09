package objectstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

type memoryObject struct {
	metadata Object
	data     []byte
}

type MemoryStore struct {
	mu                 sync.RWMutex
	objects            map[string]memoryObject
	MaximumObjectBytes int64
}

func NewMemoryStore(maximumObjectBytes int64) *MemoryStore {
	if maximumObjectBytes <= 0 {
		maximumObjectBytes = 32 << 20
	}
	return &MemoryStore{objects: make(map[string]memoryObject), MaximumObjectBytes: maximumObjectBytes}
}

func (s *MemoryStore) Put(ctx context.Context, object Object, reader io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	object.Key = strings.TrimSpace(object.Key)
	if object.Key == "" || reader == nil {
		return fmt.Errorf("%w: object key and reader are required", core.ErrInvalidArgument)
	}
	payload, err := io.ReadAll(io.LimitReader(reader, s.MaximumObjectBytes+1))
	if err != nil {
		return err
	}
	if int64(len(payload)) > s.MaximumObjectBytes {
		return core.ErrFrameTooLarge
	}
	if object.Size > 0 && object.Size != int64(len(payload)) {
		return fmt.Errorf("%w: declared object size does not match payload", core.ErrConflict)
	}
	object.Size = int64(len(payload))
	s.mu.Lock()
	s.objects[object.Key] = memoryObject{metadata: object, data: append([]byte(nil), payload...)}
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, key string) (io.ReadCloser, Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, Object{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, Object{}, fmt.Errorf("%w: object key", core.ErrInvalidArgument)
	}
	s.mu.RLock()
	stored, ok := s.objects[key]
	s.mu.RUnlock()
	if !ok {
		return nil, Object{}, core.ErrNotFound
	}
	if !stored.metadata.ExpiresAt.IsZero() && !stored.metadata.ExpiresAt.After(time.Now()) {
		_ = s.Delete(context.Background(), key)
		return nil, Object{}, core.ErrNotFound
	}
	payload := append([]byte(nil), stored.data...)
	return io.NopCloser(bytes.NewReader(payload)), stored.metadata, nil
}

func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if stored, ok := s.objects[key]; ok {
		zeroBytes(stored.data)
		delete(s.objects, key)
	}
	s.mu.Unlock()
	return nil
}

func zeroBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
