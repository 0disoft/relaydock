package credentials

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
)

// MaximumSystemCredentialBytes is the largest value accepted by native stores.
const MaximumSystemCredentialBytes = 2048

var ErrSystemStoreUnavailable = errors.New("system credential store unavailable")

type systemCredentialBackend interface {
	Write(target string, value []byte) error
	Read(target string) ([]byte, error)
	Delete(target string) error
}

// SystemStore persists credentials in the current user's native operating
// system credential store. Targets contain only a namespace and a one-way hash
// of the logical credential identity; provider and account names are not
// exposed to the operating system credential list.
type SystemStore struct {
	namespace string
	backend   systemCredentialBackend
}

var _ Store = (*SystemStore)(nil)

func NewSystemStore(namespace string) (*SystemStore, error) {
	backend, err := newPlatformCredentialBackend()
	if err != nil {
		return nil, err
	}
	return newSystemStore(namespace, backend)
}

func newSystemStore(namespace string, backend systemCredentialBackend) (*SystemStore, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" || len(namespace) > 96 {
		return nil, fmt.Errorf("%w: credential namespace must contain between 1 and 96 characters", core.ErrInvalidConfiguration)
	}
	for _, char := range namespace {
		if !(char == '.' || char == '-' || char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9') {
			return nil, fmt.Errorf("%w: credential namespace contains unsupported characters", core.ErrInvalidConfiguration)
		}
	}
	if backend == nil {
		return nil, fmt.Errorf("%w: credential backend", core.ErrInvalidConfiguration)
	}
	return &SystemStore{namespace: namespace, backend: backend}, nil
}

func (s *SystemStore) Put(ctx context.Context, ref Reference, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := s.target(ref)
	if err != nil {
		return err
	}
	if len(value) == 0 || len(value) > MaximumSystemCredentialBytes {
		return fmt.Errorf("%w: credential must contain between 1 and %d bytes", core.ErrInvalidArgument, MaximumSystemCredentialBytes)
	}
	copyValue := append([]byte(nil), value...)
	defer zeroCredential(copyValue)
	if err := s.backend.Write(target, copyValue); err != nil {
		return fmt.Errorf("write system credential: %w", err)
	}
	return nil
}

func (s *SystemStore) Get(ctx context.Context, ref Reference) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, err := s.target(ref)
	if err != nil {
		return nil, err
	}
	value, err := s.backend.Read(target)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("read system credential: %w", err)
	}
	if len(value) == 0 || len(value) > MaximumSystemCredentialBytes {
		zeroCredential(value)
		return nil, fmt.Errorf("%w: stored credential has an invalid size", core.ErrInvalidConfiguration)
	}
	result := append([]byte(nil), value...)
	zeroCredential(value)
	return result, nil
}

func (s *SystemStore) Delete(ctx context.Context, ref Reference) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := s.target(ref)
	if err != nil {
		return err
	}
	if err := s.backend.Delete(target); err != nil && !errors.Is(err, core.ErrNotFound) {
		return fmt.Errorf("delete system credential: %w", err)
	}
	return nil
}

func (s *SystemStore) target(ref Reference) (string, error) {
	if s == nil || s.backend == nil {
		return "", fmt.Errorf("%w: system credential store", core.ErrInvalidConfiguration)
	}
	key, err := credentialKey(ref)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(key))
	return s.namespace + ":" + base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func zeroCredential(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
