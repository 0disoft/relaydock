package storage_test

import (
	"context"
	"crypto/ed25519"
	"path/filepath"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/control/snapshot"
	"github.com/0disoft/relaydock/internal/transport/controlhttp"
)

func TestControlSigningKeyAndSnapshotPersistAcrossRestart(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	keyPath := filepath.Join(root, "control", "signing.key")
	statePath := filepath.Join(root, "control", "snapshot.json")

	firstSigner, err := snapshot.LoadOrCreateSigningKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	firstStore, err := snapshot.OpenLocalStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlhttp.NewAPIWithDependencies(firstStore, firstSigner, firstSigner.PublicKey); err != nil {
		t.Fatal(err)
	}
	firstSnapshot, err := firstStore.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := firstSigner.Verify(context.Background(), firstSnapshot); err != nil {
		t.Fatal(err)
	}

	secondSigner, err := snapshot.LoadOrCreateSigningKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.PublicKey(firstSigner.PublicKey).Equal(secondSigner.PublicKey) {
		t.Fatal("control signing public key changed after reopening")
	}
	secondStore, err := snapshot.OpenLocalStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlhttp.NewAPIWithDependencies(secondStore, secondSigner, secondSigner.PublicKey); err != nil {
		t.Fatal(err)
	}
	secondSnapshot, err := secondStore.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if secondSnapshot.Revision != firstSnapshot.Revision || string(secondSnapshot.Signature) != string(firstSnapshot.Signature) {
		t.Fatalf("snapshot changed across healthy restart: first=%d second=%d", firstSnapshot.Revision, secondSnapshot.Revision)
	}
}

func TestExpiredPersistedSnapshotIsRenewedWithNextRevision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	signer, err := snapshot.LoadOrCreateSigningKey(filepath.Join(root, "signing.key"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := snapshot.OpenLocalStore(filepath.Join(root, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	expired, err := signer.Sign(context.Background(), snapshot.Snapshot{
		Revision:    7,
		GeneratedAt: time.Now().UTC().Add(-2 * time.Hour),
		ExpiresAt:   time.Now().UTC().Add(-time.Hour),
		Models:      []snapshot.ModelRoute{{VirtualModel: "code-deep", Candidates: []string{"openai/model"}, PolicyID: "p1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Publish(context.Background(), expired); err != nil {
		t.Fatal(err)
	}
	if _, err := controlhttp.NewAPIWithDependencies(store, signer, signer.PublicKey); err != nil {
		t.Fatal(err)
	}
	current, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 8 || !current.ExpiresAt.After(time.Now().UTC()) {
		t.Fatalf("unexpected renewed snapshot: %+v", current)
	}
	if err := signer.Verify(context.Background(), current); err != nil {
		t.Fatal(err)
	}
}
