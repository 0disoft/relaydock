package snapshot

/* llmnav/1 file
id=relaydock.control.snapshot.contract
role=Verify snapshot rollback, same-revision mutation, and last-known-good persistence failure handling.
search=snapshot manager tests|older revision rejection|last known good failure
stability=contract
*/

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

type failingPublishStore struct{}

func (failingPublishStore) Current(context.Context) (Snapshot, error) {
	return Snapshot{}, core.ErrNotFound
}
func (failingPublishStore) Publish(context.Context, Snapshot) error { return errors.New("disk full") }
func (failingPublishStore) Watch(context.Context, int64) (<-chan Snapshot, error) {
	return make(chan Snapshot), nil
}

func TestManagerRejectsRemoteRevisionOlderThanLastKnownGood(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 8, 2, 0, 0, 0, time.UTC)
	signer := NewEd25519Signer(private, public)
	lastKnown := mustSignedSnapshot(t, signer, Snapshot{
		Revision:    5,
		GeneratedAt: now,
		ExpiresAt:   now.Add(time.Hour),
		Models:      []ModelRoute{{VirtualModel: "code", Candidates: []string{"openai/new"}}},
	})
	remote := mustSignedSnapshot(t, signer, Snapshot{
		Revision:    4,
		GeneratedAt: now,
		ExpiresAt:   now.Add(time.Hour),
		Models:      []ModelRoute{{VirtualModel: "code", Candidates: []string{"openai/old"}}},
	})
	store := NewMemoryStore()
	if err := store.Publish(context.Background(), lastKnown); err != nil {
		t.Fatal(err)
	}
	client, err := NewRemoteClient("https://control.invalid", "", public, nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Now = func() time.Time { return now }
	applied := false
	manager := &Manager{
		Client:    client,
		LastKnown: store,
		Now:       func() time.Time { return now },
		Apply: func(context.Context, Snapshot) error {
			applied = true
			return nil
		},
	}
	if err := manager.applyVerified(context.Background(), remote, false); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("expected rollback conflict, got %v", err)
	}
	if applied {
		t.Fatal("older remote snapshot was applied")
	}
}

func TestManagerKeepsVerifiedSnapshotActiveWhenLastKnownPersistenceFails(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 8, 2, 0, 0, 0, time.UTC)
	signer := NewEd25519Signer(private, public)
	value := mustSignedSnapshot(t, signer, Snapshot{
		Revision:    1,
		GeneratedAt: now,
		ExpiresAt:   now.Add(time.Hour),
		Models:      []ModelRoute{{VirtualModel: "code", Candidates: []string{"openai/model"}}},
	})
	client, err := NewRemoteClient("https://control.invalid", "", public, nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Now = func() time.Time { return now }
	applied := int64(0)
	manager := &Manager{
		Client:    client,
		LastKnown: failingPublishStore{},
		Now:       func() time.Time { return now },
		Apply: func(_ context.Context, snapshot Snapshot) error {
			applied = snapshot.Revision
			return nil
		},
	}
	if err := manager.applyVerified(context.Background(), value, false); err != nil {
		t.Fatalf("active verified snapshot should survive persistence degradation: %v", err)
	}
	status := manager.Status()
	if applied != 1 || status.Revision != 1 || !manager.Ready() || status.LastError == "" {
		t.Fatalf("unexpected degraded status: applied=%d status=%+v", applied, status)
	}
	current, ok := manager.Current()
	if !ok || current.Revision != 1 {
		t.Fatalf("current snapshot missing: ok=%v value=%+v", ok, current)
	}
}

func TestManagerRejectsContentMutationWithoutRevisionIncrement(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 8, 2, 0, 0, 0, time.UTC)
	signer := NewEd25519Signer(private, public)
	first := mustSignedSnapshot(t, signer, Snapshot{
		Revision:    9,
		GeneratedAt: now,
		ExpiresAt:   now.Add(time.Hour),
		Models:      []ModelRoute{{VirtualModel: "code", Candidates: []string{"openai/a"}}},
	})
	mutated := mustSignedSnapshot(t, signer, Snapshot{
		Revision:    9,
		GeneratedAt: now,
		ExpiresAt:   now.Add(time.Hour),
		Models:      []ModelRoute{{VirtualModel: "code", Candidates: []string{"openai/b"}}},
	})
	client, err := NewRemoteClient("https://control.invalid", "", public, nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Now = func() time.Time { return now }
	manager := &Manager{Client: client, Now: func() time.Time { return now }, Apply: func(context.Context, Snapshot) error { return nil }}
	if err := manager.applyVerified(context.Background(), first, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.applyVerified(context.Background(), mutated, false); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("expected same-revision conflict, got %v", err)
	}
	current, _ := manager.Current()
	if current.Models[0].Candidates[0] != "openai/a" {
		t.Fatalf("current snapshot was mutated: %+v", current)
	}
}

type transientPublishStore struct {
	value      Snapshot
	hasValue   bool
	failWrites int
}

func (s *transientPublishStore) Current(context.Context) (Snapshot, error) {
	if !s.hasValue {
		return Snapshot{}, core.ErrNotFound
	}
	return cloneSnapshot(s.value), nil
}

func (s *transientPublishStore) Publish(_ context.Context, value Snapshot) error {
	if s.failWrites > 0 {
		s.failWrites--
		return errors.New("temporary disk failure")
	}
	s.value = cloneSnapshot(value)
	s.hasValue = true
	return nil
}

func (*transientPublishStore) Watch(context.Context, int64) (<-chan Snapshot, error) {
	return make(chan Snapshot), nil
}

func TestManagerRetriesLastKnownPersistenceForSameActiveRevision(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 8, 3, 0, 0, 0, time.UTC)
	signer := NewEd25519Signer(private, public)
	value := mustSignedSnapshot(t, signer, Snapshot{
		Revision:    12,
		GeneratedAt: now,
		ExpiresAt:   now.Add(time.Hour),
		Models:      []ModelRoute{{VirtualModel: "code", Candidates: []string{"openai/model"}}},
	})
	client, err := NewRemoteClient("https://control.invalid", "", public, nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Now = func() time.Time { return now }
	store := &transientPublishStore{failWrites: 1}
	applyCount := 0
	manager := &Manager{
		Client:    client,
		LastKnown: store,
		Now:       func() time.Time { return now },
		Apply: func(context.Context, Snapshot) error {
			applyCount++
			return nil
		},
	}

	if err := manager.applyVerified(context.Background(), value, false); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if status := manager.Status(); status.LastError == "" || status.LastKnownRevision != 0 {
		t.Fatalf("expected degraded persistence state, got %+v", status)
	}
	if err := manager.applyVerified(context.Background(), value, false); err != nil {
		t.Fatalf("same-revision retry: %v", err)
	}
	status := manager.Status()
	if status.LastError != "" || status.LastKnownRevision != value.Revision || !store.hasValue {
		t.Fatalf("same-revision retry did not repair last-known-good state: %+v", status)
	}
	if applyCount != 1 {
		t.Fatalf("same revision was applied twice: %d", applyCount)
	}
}
