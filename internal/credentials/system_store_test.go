package credentials

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/relaydock/internal/core"
)

type memorySystemBackend struct {
	values map[string][]byte
	err    error
}

func newMemorySystemBackend() *memorySystemBackend {
	return &memorySystemBackend{values: make(map[string][]byte)}
}

func (b *memorySystemBackend) Write(_ context.Context, target string, value []byte) error {
	if b.err != nil {
		return b.err
	}
	b.values[target] = append([]byte(nil), value...)
	return nil
}

func (b *memorySystemBackend) Read(_ context.Context, target string) ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}
	value, ok := b.values[target]
	if !ok {
		return nil, core.ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (b *memorySystemBackend) Delete(_ context.Context, target string) error {
	if b.err != nil {
		return b.err
	}
	if _, ok := b.values[target]; !ok {
		return core.ErrNotFound
	}
	delete(b.values, target)
	return nil
}

func TestSystemStoreRoundTripUsesOpaqueTargetAndCopiesValues(t *testing.T) {
	backend := newMemorySystemBackend()
	store, err := newSystemStore("com.0disoft.relaydock.credentials.v1", backend)
	if err != nil {
		t.Fatal(err)
	}
	ref := Reference{ID: "primary", Provider: "OpenAI", Account: "team@example.com"}
	credential := []byte("sk-example-secret")
	if err := store.Put(context.Background(), ref, credential); err != nil {
		t.Fatal(err)
	}
	credential[0] = 'x'
	if len(backend.values) != 1 {
		t.Fatalf("stored targets=%d, want 1", len(backend.values))
	}
	for target, value := range backend.values {
		if strings.Contains(target, "openai") || strings.Contains(target, "team") || strings.Contains(target, "primary") {
			t.Fatalf("credential target exposes logical identity: %q", target)
		}
		if !bytes.Equal(value, []byte("sk-example-secret")) {
			t.Fatalf("stored value changed through caller memory: %q", value)
		}
	}
	first, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	first[0] = 'x'
	second, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second, []byte("sk-example-secret")) {
		t.Fatalf("resolved value changed through caller memory: %q", second)
	}
	if err := store.Delete(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), ref); err != nil {
		t.Fatalf("idempotent delete failed: %v", err)
	}
	if _, err := store.Get(context.Background(), ref); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("missing credential error=%v, want ErrNotFound", err)
	}
}

func TestSystemStoreValidatesNamespaceIdentitySizeAndContext(t *testing.T) {
	backend := newMemorySystemBackend()
	invalidNamespaces := []string{"", "contains space", strings.Repeat("a", 97)}
	for _, namespace := range invalidNamespaces {
		if _, err := newSystemStore(namespace, backend); err == nil {
			t.Fatalf("accepted namespace %q", namespace)
		}
	}
	store, err := newSystemStore("relaydock.credentials.v1", backend)
	if err != nil {
		t.Fatal(err)
	}
	ref := Reference{ID: "primary", Provider: "openai"}
	if err := store.Put(context.Background(), Reference{}, []byte("secret")); err == nil {
		t.Fatal("accepted empty credential identity")
	}
	if err := store.Put(context.Background(), ref, nil); err == nil {
		t.Fatal("accepted empty credential")
	}
	if err := store.Put(context.Background(), ref, bytes.Repeat([]byte{'x'}, MaximumSystemCredentialBytes+1)); err == nil {
		t.Fatal("accepted oversized credential")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Put(canceled, ref, []byte("secret")); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled write error=%v", err)
	}
}

func TestSystemStorePreservesBackendFailureWithoutSecretMaterial(t *testing.T) {
	backend := newMemorySystemBackend()
	backend.err = errors.New("keychain locked")
	store, err := newSystemStore("relaydock.credentials.v1", backend)
	if err != nil {
		t.Fatal(err)
	}
	secret := "super-secret-value"
	err = store.Put(context.Background(), Reference{ID: "primary", Provider: "openai"}, []byte(secret))
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe backend error: %v", err)
	}
}
