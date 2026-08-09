package controlhttp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/control/snapshot"
)

func TestControlStartupResignsUsableSnapshotWhenActiveKeyRotates(t *testing.T) {
	t.Parallel()
	oldPublic, oldPrivate, _ := ed25519.GenerateKey(rand.Reader)
	newPublic, newPrivate, _ := ed25519.GenerateKey(rand.Reader)
	oldSigner := snapshot.NewEd25519SignerWithKeyID("old", oldPrivate, oldPublic)
	newSigner := snapshot.NewEd25519SignerWithKeyID("new", newPrivate, newPublic)
	store := snapshot.NewMemoryStore()
	value, err := oldSigner.Sign(context.Background(), snapshot.Snapshot{
		Revision: 7, GeneratedAt: time.Now().Add(-time.Minute).UTC(), ExpiresAt: time.Now().Add(time.Hour).UTC(),
		Models: []snapshot.ModelRoute{{VirtualModel: "code", Candidates: []string{"openai/model"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Publish(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	ring, err := snapshot.NewEd25519KeyRing([]snapshot.TrustedPublicKey{
		{ID: "old", PublicKey: oldPublic}, {ID: "new", PublicKey: newPublic},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewAPIWithVerifier(store, newSigner, ring, newPublic); err != nil {
		t.Fatal(err)
	}
	current, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 8 || current.SigningKeyID != "new" {
		t.Fatalf("snapshot was not rotated: %#v", current)
	}
	if err := newSigner.Verify(context.Background(), current); err != nil {
		t.Fatal(err)
	}
}
