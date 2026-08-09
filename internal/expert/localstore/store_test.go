package localstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	"github.com/your-org/ai-runtime-gateway/internal/expert/resultcontract"
)

func TestContextPackContentUsesBoundedContentAddressedChunks(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "expert.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("0123456789abcdef", 9_000)
	pack := testPack("ctx_large", content, time.Now().UTC())
	if err := store.Put(context.Background(), pack); err != nil {
		t.Fatal(err)
	}

	stateRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stateRaw), content[:256]) {
		t.Fatal("metadata file embeds ContextPack source content")
	}
	chunkCount := 0
	if err := filepath.WalkDir(path+".chunks", func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		chunkCount++
		if info.Size() > defaultContentChunk {
			t.Fatalf("chunk exceeds bound: %d", info.Size())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if chunkCount < 2 {
		t.Fatalf("expected multiple chunks, got %d", chunkCount)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	hydrated, err := reopened.GetPack(context.Background(), pack.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := hydrated.Evidence[0].Content; got != content {
		t.Fatalf("hydrated content mismatch: %d != %d", len(got), len(content))
	}
}

func TestOpenMigratesLegacyEmbeddedPacksToVersionThree(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "expert.json")
	content := strings.Repeat("legacy-content\n", 5_000)
	pack := testPack("ctx_legacy", content, time.Now().Add(-time.Hour).UTC())
	legacy := legacyState{
		Version:        2,
		Consultations:  map[string]consultation.Consultation{},
		Idempotency:    map[string]idempotencyRecord{},
		ContextPacks:   map[string]contextpack.Pack{pack.ID: pack},
		Results:        map[string]resultcontract.Result{},
		ResultMetadata: map[string]resultMetadata{},
	}
	raw, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".v2.bak"); err != nil {
		t.Fatalf("legacy backup missing: %v", err)
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(persisted, &header); err != nil {
		t.Fatal(err)
	}
	if header.Version != currentVersion {
		t.Fatalf("version=%d", header.Version)
	}
	hydrated, err := store.GetPack(context.Background(), pack.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hydrated.Evidence[0].Content != content {
		t.Fatal("legacy content was not preserved")
	}
}

func TestCompactRemovesExpiredTerminalGraphAndOrphanChunks(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "expert.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now.Add(-48 * time.Hour) }
	pack := testPack("ctx_old", strings.Repeat("old", 20_000), store.now())
	created, _, err := store.CreateWithContext(context.Background(), pack, consultation.CreateCommand{
		Objective: "old", ContextPackID: pack.ID, IdempotencyKey: "old", TTL: 72 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateState(context.Background(), created.ID, consultation.StateApprovalPending, consultation.StateCancelled); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now }
	report, err := store.Compact(context.Background(), CompactPolicy{
		Now: time.Now().UTC().Add(time.Hour), TerminalRetention: 24 * time.Hour, RemoveOrphans: true, OrphanGracePeriod: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.ConsultationsRemoved != 1 || report.ContextPacksRemoved != 1 || report.IdempotencyRemoved != 1 || report.ChunksRemoved == 0 {
		t.Fatalf("unexpected compaction report: %#v", report)
	}
	if _, err := store.Get(context.Background(), created.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("consultation still exists: %v", err)
	}
	if _, err := store.GetPack(context.Background(), pack.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("pack still exists: %v", err)
	}
}

func TestCreateWithContextIsAtomicAndIdempotent(t *testing.T) {
	t.Parallel()
	store, err := Open(filepath.Join(t.TempDir(), "expert.json"))
	if err != nil {
		t.Fatal(err)
	}
	pack := testPack("ctx_atomic", "package main\n", time.Now().UTC())
	command := consultation.CreateCommand{Objective: "review", IdempotencyKey: "same", TTL: time.Hour}
	first, firstPack, err := store.CreateWithContext(context.Background(), pack, command)
	if err != nil {
		t.Fatal(err)
	}
	second, secondPack, err := store.CreateWithContext(context.Background(), pack, command)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || firstPack.ID != secondPack.ID {
		t.Fatalf("idempotent create diverged: %#v %#v", first, second)
	}
	changed := pack
	changed.Objective = "different pack"
	if _, _, err := store.CreateWithContext(context.Background(), changed, consultation.CreateCommand{Objective: "other", IdempotencyKey: "other"}); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("expected immutable pack conflict, got %v", err)
	}
}

func TestOpenRejectsSameSizeChunkTampering(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "expert.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	pack := testPack("ctx_tampered", strings.Repeat("integrity", 8_000), time.Now().UTC())
	if err := store.Put(context.Background(), pack); err != nil {
		t.Fatal(err)
	}
	var chunkPath string
	if err := filepath.WalkDir(path+".chunks", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && chunkPath == "" {
			chunkPath = path
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if chunkPath == "" {
		t.Fatal("chunk was not written")
	}
	raw, err := os.ReadFile(chunkPath)
	if err != nil {
		t.Fatal(err)
	}
	raw[0] ^= 0xff
	if err := os.WriteFile(chunkPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "corrupt content chunk") {
		t.Fatalf("expected full chunk integrity rejection, got %v", err)
	}
}

func TestConcurrentContextCreationAndCompactionPreserveReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "expert.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	const count = 40
	var wait sync.WaitGroup
	errorsChannel := make(chan error, count*2)
	wait.Add(2)
	go func() {
		defer wait.Done()
		for index := 0; index < count; index++ {
			id := fmt.Sprintf("ctx_concurrent_%03d", index)
			pack := testPack(id, strings.Repeat(id, 5_000), time.Now().UTC())
			_, _, err := store.CreateWithContext(context.Background(), pack, consultation.CreateCommand{
				Objective: "concurrent compaction safety", IdempotencyKey: id, TTL: time.Hour,
			})
			if err != nil {
				errorsChannel <- err
				return
			}
		}
	}()
	go func() {
		defer wait.Done()
		for index := 0; index < count; index++ {
			_, err := store.Compact(context.Background(), CompactPolicy{
				Now: time.Now().UTC().Add(time.Hour), RemoveOrphans: true, OrphanGracePeriod: 0,
			})
			if err != nil {
				errorsChannel <- err
				return
			}
		}
	}()
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := reopened.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.ContextPacks != count || stats.Consultations != count {
		t.Fatalf("concurrent operations lost references: %#v", stats)
	}
}

func testPack(id, content string, createdAt time.Time) contextpack.Pack {
	return contextpack.Pack{
		ID: id, Objective: "review", RepositoryRoot: "/repo", CreatedAt: createdAt, ExpiresAt: createdAt.Add(24 * time.Hour),
		Evidence: []contextpack.Evidence{{Reference: "main.go", Content: content, Digest: contextpack.Digest([]byte(content)), Bytes: int64(len(content))}},
	}
}
