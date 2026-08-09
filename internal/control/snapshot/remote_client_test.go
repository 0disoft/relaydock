package snapshot

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

func TestRemoteClientFetchAndWatch(t *testing.T) {
	t.Parallel()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	signer := NewEd25519Signer(private, public)
	first := mustSignedSnapshot(t, signer, Snapshot{
		Revision:    7,
		GeneratedAt: now,
		ExpiresAt:   now.Add(time.Hour),
		Models:      []ModelRoute{{VirtualModel: "code-deep", Candidates: []string{"openai/gpt"}}},
	})
	second := mustSignedSnapshot(t, signer, Snapshot{
		Revision:    8,
		GeneratedAt: now.Add(time.Minute),
		ExpiresAt:   now.Add(2 * time.Hour),
		Models:      []ModelRoute{{VirtualModel: "code-deep", Candidates: []string{"anthropic/opus"}}},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/snapshot":
			if r.Header.Get("Authorization") != "Bearer control-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("ETag", `"snapshot-7"`)
			if r.Header.Get("If-None-Match") == `"snapshot-7"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			_ = json.NewEncoder(w).Encode(first)
		case "/v1/snapshots/watch":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher := w.(http.Flusher)
			raw, _ := json.Marshal(second)
			_, _ = fmt.Fprintf(w, ": keepalive\n\nid: 8\nevent: snapshot\ndata: %s\n\n", raw)
			flusher.Flush()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewRemoteClient(server.URL, "control-token", public, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Now = func() time.Time { return now }
	value, etag, err := client.FetchCurrent(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if value.Revision != 7 || etag != `"snapshot-7"` {
		t.Fatalf("unexpected fetched snapshot: revision=%d etag=%q", value.Revision, etag)
	}
	if _, _, err := client.FetchCurrent(context.Background(), etag); !errors.Is(err, core.ErrNotModified) {
		t.Fatalf("conditional fetch error = %v, want ErrNotModified", err)
	}

	var watched Snapshot
	err = client.Watch(context.Background(), 7, func(_ context.Context, next Snapshot) error {
		watched = next
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("watch error = %v, want context.Canceled", err)
	}
	if watched.Revision != 8 {
		t.Fatalf("watched revision = %d, want 8", watched.Revision)
	}
}

func TestManagerFallsBackToLastKnownGood(t *testing.T) {
	t.Parallel()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 8, 1, 0, 0, 0, time.UTC)
	signer := NewEd25519Signer(private, public)
	lastKnown := mustSignedSnapshot(t, signer, Snapshot{
		Revision:    3,
		GeneratedAt: now.Add(-time.Minute),
		ExpiresAt:   now.Add(time.Hour),
		Models:      []ModelRoute{{VirtualModel: "local/echo", Candidates: []string{"local/echo"}}},
	})
	store := NewMemoryStore()
	if err := store.Publish(context.Background(), lastKnown); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, err := NewRemoteClient(server.URL, "", public, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Now = func() time.Time { return now }

	var mu sync.Mutex
	applied := int64(0)
	manager := &Manager{
		Client:    client,
		LastKnown: store,
		Required:  true,
		Now:       func() time.Time { return now },
		Apply: func(_ context.Context, value Snapshot) error {
			mu.Lock()
			defer mu.Unlock()
			applied = value.Revision
			return nil
		},
	}
	if err := manager.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := applied
	mu.Unlock()
	if got != 3 || !manager.Ready() || !manager.Status().UsingLastKnown {
		t.Fatalf("fallback status = %#v, applied=%d", manager.Status(), got)
	}
}

func TestValidateRejectsExpiredAndDuplicateRoutes(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	expired := Snapshot{
		Revision:    1,
		GeneratedAt: now.Add(-2 * time.Hour),
		ExpiresAt:   now.Add(-time.Hour),
		Models:      []ModelRoute{{VirtualModel: "x", Candidates: []string{"a/b"}}},
	}
	if err := Validate(expired, ValidationOptions{Now: now, RequireUnexpired: true}); !errors.Is(err, core.ErrExpired) {
		t.Fatalf("expired validation error = %v", err)
	}
	duplicate := Snapshot{
		Revision:    2,
		GeneratedAt: now,
		ExpiresAt:   now.Add(time.Hour),
		Models: []ModelRoute{
			{VirtualModel: "x", Candidates: []string{"a/b"}},
			{VirtualModel: "x", Candidates: []string{"c/d"}},
		},
	}
	if err := Validate(duplicate, ValidationOptions{Now: now, RequireUnexpired: true}); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("duplicate validation error = %v", err)
	}
}

func mustSignedSnapshot(t *testing.T, signer Ed25519Signer, value Snapshot) Snapshot {
	t.Helper()
	signed, err := signer.Sign(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestValidateRejectsMalformedPriceRevisions(t *testing.T) {
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	base := Snapshot{
		Revision: 1, GeneratedAt: now, ExpiresAt: now.Add(time.Hour),
		Models: []ModelRoute{{VirtualModel: "code", Candidates: []string{"openai/model"}}},
	}
	missing := base
	missing.PriceRevisions = []PriceRevision{{ID: "price"}}
	if err := Validate(missing, ValidationOptions{Now: now}); err == nil {
		t.Fatal("price revision without effectiveAt was accepted")
	}
	futureOnly := base
	futureOnly.PriceRevisions = []PriceRevision{{ID: "future", EffectiveAt: now.Add(time.Hour)}}
	if err := Validate(futureOnly, ValidationOptions{Now: now}); err == nil {
		t.Fatal("snapshot without an active price revision was accepted")
	}
	duplicate := base
	duplicate.PriceRevisions = []PriceRevision{
		{ID: "price", EffectiveAt: now},
		{ID: "price", EffectiveAt: now.Add(time.Minute)},
	}
	if err := Validate(duplicate, ValidationOptions{Now: now}); err == nil {
		t.Fatal("duplicate price revision was accepted")
	}
}
