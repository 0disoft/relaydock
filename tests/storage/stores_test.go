package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/credentials"
	"github.com/your-org/ai-runtime-gateway/internal/outbox"
	"github.com/your-org/ai-runtime-gateway/internal/persistence/objectstore"
	"github.com/your-org/ai-runtime-gateway/internal/security/secrets"
)

func TestCredentialAndSecretStoresIsolateCallerMemory(t *testing.T) {
	ctx := context.Background()
	credentialStore := credentials.NewMemoryStore()
	credentialRef := credentials.Reference{ID: "primary", Provider: "openai", Account: "team"}
	credential := []byte("initial-api-key")
	if err := credentialStore.Put(ctx, credentialRef, credential); err != nil {
		t.Fatal(err)
	}
	credential[0] = 'X'
	stored, err := credentialStore.Get(ctx, credentialRef)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != "initial-api-key" {
		t.Fatalf("caller mutated stored credential: %q", stored)
	}
	stored[0] = 'Y'
	again, _ := credentialStore.Get(ctx, credentialRef)
	if string(again) != "initial-api-key" {
		t.Fatalf("read buffer mutated credential store: %q", again)
	}

	secretStore, err := secrets.NewEncryptedMemoryStore(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	secretRef := secrets.Reference{ID: "provider-key", TenantID: "tenant-a", Kind: "api-key"}
	if err := secretStore.Put(ctx, secretRef, []byte("server-side-secret")); err != nil {
		t.Fatal(err)
	}
	resolved, err := secretStore.Resolve(ctx, secretRef)
	if err != nil || string(resolved) != "server-side-secret" {
		t.Fatalf("resolve secret: value=%q error=%v", resolved, err)
	}
	if _, err := secretStore.Resolve(ctx, secrets.Reference{ID: "provider-key", TenantID: "tenant-b", Kind: "api-key"}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("cross-tenant secret lookup was not isolated: %v", err)
	}
	if err := secretStore.Delete(ctx, secretRef); err != nil {
		t.Fatal(err)
	}
	if _, err := secretStore.Resolve(ctx, secretRef); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("deleted secret still resolves: %v", err)
	}
}

func TestObjectStoreEnforcesSizeAndExpiry(t *testing.T) {
	ctx := context.Background()
	store := objectstore.NewMemoryStore(16)
	if err := store.Put(ctx, objectstore.Object{Key: "pack", ExpiresAt: time.Now().Add(time.Minute)}, bytes.NewBufferString("context")); err != nil {
		t.Fatal(err)
	}
	reader, metadata, err := store.Get(ctx, "pack")
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(payload) != "context" || metadata.Size != 7 {
		t.Fatalf("unexpected object: metadata=%+v payload=%q", metadata, payload)
	}
	if err := store.Put(ctx, objectstore.Object{Key: "too-large"}, bytes.NewBufferString("0123456789abcdefg")); !errors.Is(err, core.ErrFrameTooLarge) {
		t.Fatalf("oversized object accepted: %v", err)
	}
	if err := store.Put(ctx, objectstore.Object{Key: "expired", ExpiresAt: time.Now().Add(-time.Second)}, bytes.NewBufferString("old")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(ctx, "expired"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expired object still readable: %v", err)
	}
}

func TestOutboxClaimsAvailableEventsInOrder(t *testing.T) {
	repository := outbox.NewMemoryRepository()
	now := time.Now().UTC()
	for _, event := range []outbox.Event{
		{ID: "later", Topic: "usage", Payload: json.RawMessage(`{"n":2}`), AvailableAt: now.Add(time.Minute)},
		{ID: "second", Topic: "usage", Payload: json.RawMessage(`{"n":1}`), AvailableAt: now.Add(-time.Second)},
		{ID: "first", Topic: "usage", Payload: json.RawMessage(`{"n":0}`), AvailableAt: now.Add(-2 * time.Second)},
	} {
		if err := repository.Enqueue(event); err != nil {
			t.Fatal(err)
		}
	}
	claimed := repository.Claim(now, 10)
	if len(claimed) != 2 || claimed[0].ID != "first" || claimed[1].ID != "second" {
		t.Fatalf("unexpected claim order: %+v", claimed)
	}
	remaining := repository.Snapshot()
	if len(remaining) != 1 || remaining[0].ID != "later" {
		t.Fatalf("future event was claimed: %+v", remaining)
	}
	if err := repository.Requeue(claimed[0], now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	requeued := repository.Snapshot()
	if len(requeued) != 2 {
		t.Fatalf("requeued event missing: %+v", requeued)
	}
	for _, event := range requeued {
		if event.ID == "first" && event.Attempts != 1 {
			t.Fatalf("requeue attempt count not incremented: %+v", event)
		}
	}
}
