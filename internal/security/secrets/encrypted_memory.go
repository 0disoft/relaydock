package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

type encryptedValue struct {
	nonce      []byte
	ciphertext []byte
}

// EncryptedMemoryStore keeps ciphertext rather than plaintext at rest in the
// process heap. Production deployments should replace it with an OS keychain or
// KMS-backed store while preserving this Store contract.
type EncryptedMemoryStore struct {
	mu     sync.RWMutex
	aead   cipher.AEAD
	values map[string]encryptedValue
}

func NewEncryptedMemoryStore(masterKey []byte) (*EncryptedMemoryStore, error) {
	if len(masterKey) == 0 {
		masterKey = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, masterKey); err != nil {
			return nil, fmt.Errorf("generate secret-store key: %w", err)
		}
	}
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("%w: secret-store key must be 32 bytes", core.ErrInvalidConfiguration)
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &EncryptedMemoryStore{aead: aead, values: make(map[string]encryptedValue)}, nil
}

func (s *EncryptedMemoryStore) Put(ctx context.Context, ref Reference, plaintext []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key, aad, err := secretIdentity(ref)
	if err != nil {
		return err
	}
	if len(plaintext) == 0 {
		return fmt.Errorf("%w: empty secret", core.ErrInvalidArgument)
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generate secret nonce: %w", err)
	}
	ciphertext := s.aead.Seal(nil, nonce, plaintext, aad)
	s.mu.Lock()
	s.values[key] = encryptedValue{nonce: nonce, ciphertext: ciphertext}
	s.mu.Unlock()
	return nil
}

func (s *EncryptedMemoryStore) Resolve(ctx context.Context, ref Reference) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, aad, err := secretIdentity(ref)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	value, ok := s.values[key]
	if ok {
		value.nonce = append([]byte(nil), value.nonce...)
		value.ciphertext = append([]byte(nil), value.ciphertext...)
	}
	s.mu.RUnlock()
	if !ok {
		return nil, core.ErrNotFound
	}
	plaintext, err := s.aead.Open(nil, value.nonce, value.ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("%w: secret authentication failed", core.ErrUnauthorized)
	}
	return plaintext, nil
}

func (s *EncryptedMemoryStore) Delete(ctx context.Context, ref Reference) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key, _, err := secretIdentity(ref)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if value, ok := s.values[key]; ok {
		zero(value.nonce)
		zero(value.ciphertext)
		delete(s.values, key)
	}
	s.mu.Unlock()
	return nil
}

func secretIdentity(ref Reference) (key string, aad []byte, err error) {
	id := strings.TrimSpace(ref.ID)
	tenant := strings.TrimSpace(ref.TenantID)
	kind := strings.ToLower(strings.TrimSpace(ref.Kind))
	if id == "" || tenant == "" || kind == "" {
		return "", nil, fmt.Errorf("%w: secret id, tenant, and kind are required", core.ErrInvalidArgument)
	}
	key = tenant + "\x00" + kind + "\x00" + id
	return key, []byte(key), nil
}

func zero(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
