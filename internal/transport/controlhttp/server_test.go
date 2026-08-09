package controlhttp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/control/snapshot"
	"github.com/0disoft/relaydock/internal/core"
)

type commitThenConflictStore struct {
	inner snapshot.Store
	once  sync.Once
}

func (s *commitThenConflictStore) Current(ctx context.Context) (snapshot.Snapshot, error) {
	return s.inner.Current(ctx)
}

func (s *commitThenConflictStore) Publish(ctx context.Context, value snapshot.Snapshot) error {
	conflicted := false
	s.once.Do(func() {
		conflicted = true
		if err := s.inner.Publish(ctx, value); err != nil {
			panic(err)
		}
	})
	if conflicted {
		return core.ErrConflict
	}
	return s.inner.Publish(ctx, value)
}

func (s *commitThenConflictStore) Watch(ctx context.Context, after int64) (<-chan snapshot.Snapshot, error) {
	return s.inner.Watch(ctx, after)
}

func TestNewAPIRecoversWhenAnotherInstanceInitializesSharedStore(t *testing.T) {
	signer := testSigner(t)
	inner := snapshot.NewMemoryStore()
	store := &commitThenConflictStore{inner: inner}
	if _, err := NewAPIWithDependencies(store, signer, signer.PublicKey); err != nil {
		t.Fatalf("construct API after concurrent initialization: %v", err)
	}
	current, err := store.Current(context.Background())
	if err != nil {
		t.Fatalf("load initialized snapshot: %v", err)
	}
	if current.Revision != 1 || !current.ExpiresAt.After(time.Now()) {
		t.Fatalf("unexpected initialized snapshot: %#v", current)
	}
	if err := signer.Verify(context.Background(), current); err != nil {
		t.Fatalf("verify initialized snapshot: %v", err)
	}
}

func TestNewAPIRecoversWhenAnotherInstanceRenewsExpiredSnapshot(t *testing.T) {
	signer := testSigner(t)
	inner := snapshot.NewMemoryStore()
	expired, err := signer.Sign(context.Background(), snapshot.Snapshot{
		Revision:    7,
		GeneratedAt: time.Now().UTC().Add(-2 * time.Hour),
		ExpiresAt:   time.Now().UTC().Add(-time.Hour),
		Models: []snapshot.ModelRoute{{
			VirtualModel: "code-deep",
			Candidates:   []string{"openai/model"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inner.Publish(context.Background(), expired); err != nil {
		t.Fatal(err)
	}
	store := &commitThenConflictStore{inner: inner}
	if _, err := NewAPIWithDependencies(store, signer, signer.PublicKey); err != nil {
		t.Fatalf("construct API after concurrent renewal: %v", err)
	}
	current, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 8 || !current.ExpiresAt.After(time.Now()) {
		t.Fatalf("unexpected renewed snapshot: %#v", current)
	}
	if err := signer.Verify(context.Background(), current); err != nil {
		t.Fatalf("verify renewed snapshot: %v", err)
	}
}

func testSigner(t *testing.T) snapshot.Ed25519Signer {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.NewEd25519Signer(private, public)
}
