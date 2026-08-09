package localstore

import (
	"context"
	"fmt"
	"os"
	"time"
)

type CompactPolicy struct {
	Now               time.Time
	TerminalRetention time.Duration
	OrphanGracePeriod time.Duration
	RemoveOrphans     bool
	DryRun            bool
}

type CompactReport struct {
	ConsultationsRemoved int   `json:"consultationsRemoved"`
	IdempotencyRemoved   int   `json:"idempotencyRemoved"`
	ContextPacksRemoved  int   `json:"contextPacksRemoved"`
	ResultsRemoved       int   `json:"resultsRemoved"`
	ChunksRemoved        int   `json:"chunksRemoved"`
	ChunkBytesRemoved    int64 `json:"chunkBytesRemoved"`
}

type Stats struct {
	Consultations int   `json:"consultations"`
	ContextPacks  int   `json:"contextPacks"`
	Results       int   `json:"results"`
	Chunks        int   `json:"chunks"`
	StateBytes    int64 `json:"stateBytes"`
	ChunkBytes    int64 `json:"chunkBytes"`
}

func (s *Store) Compact(ctx context.Context, policy CompactPolicy) (CompactReport, error) {
	s.chunkMu.Lock()
	defer s.chunkMu.Unlock()
	if err := ctx.Err(); err != nil {
		return CompactReport{}, err
	}
	now := policy.Now.UTC()
	if now.IsZero() {
		now = s.nowUTC()
	}
	if policy.OrphanGracePeriod < 0 || policy.TerminalRetention < 0 {
		return CompactReport{}, fmt.Errorf("compaction durations must not be negative")
	}
	var report CompactReport
	var liveChunks map[string]struct{}
	if policy.DryRun {
		s.mu.RLock()
		next, err := cloneState(s.data)
		s.mu.RUnlock()
		if err != nil {
			return CompactReport{}, err
		}
		report = compactState(&next, policy, now)
		liveChunks = collectLiveChunks(next)
	} else {
		err := s.update(ctx, func(next *state) error {
			report = compactState(next, policy, now)
			liveChunks = collectLiveChunks(*next)
			return nil
		})
		if err != nil {
			return CompactReport{}, err
		}
	}
	olderThan := now.Add(-policy.OrphanGracePeriod)
	removed, removedBytes, err := s.chunks.garbageCollect(liveChunks, olderThan, policy.DryRun)
	if err != nil {
		return report, err
	}
	report.ChunksRemoved = removed
	report.ChunkBytesRemoved = removedBytes
	return report, nil
}

func compactState(next *state, policy CompactPolicy, now time.Time) CompactReport {
	var report CompactReport
	if policy.TerminalRetention > 0 {
		cutoff := now.Add(-policy.TerminalRetention)
		for id, item := range next.Consultations {
			if item.Terminal() && !item.UpdatedAt.IsZero() && !item.UpdatedAt.After(cutoff) {
				delete(next.Consultations, id)
				report.ConsultationsRemoved++
			}
		}
	}
	for scope, record := range next.Idempotency {
		if _, exists := next.Consultations[record.ConsultationID]; !exists {
			delete(next.Idempotency, scope)
			report.IdempotencyRemoved++
		}
	}
	if !policy.RemoveOrphans {
		return report
	}
	referencedPacks := make(map[string]struct{})
	referencedResults := make(map[string]struct{})
	for _, item := range next.Consultations {
		if item.ContextPackID != "" {
			referencedPacks[item.ContextPackID] = struct{}{}
		}
		if item.ResultID != "" {
			referencedResults[item.ResultID] = struct{}{}
		}
	}
	cutoff := now.Add(-policy.OrphanGracePeriod)
	for id, pack := range next.ContextPacks {
		if _, referenced := referencedPacks[id]; referenced {
			continue
		}
		createdAt := pack.Manifest.CreatedAt
		if !createdAt.IsZero() && createdAt.After(cutoff) {
			continue
		}
		delete(next.ContextPacks, id)
		report.ContextPacksRemoved++
	}
	for id := range next.Results {
		if _, referenced := referencedResults[id]; referenced {
			continue
		}
		metadata := next.ResultMetadata[id]
		if !metadata.CreatedAt.IsZero() && metadata.CreatedAt.After(cutoff) {
			continue
		}
		delete(next.Results, id)
		delete(next.ResultMetadata, id)
		report.ResultsRemoved++
	}
	return report
}

func collectLiveChunks(value state) map[string]struct{} {
	live := make(map[string]struct{})
	for _, pack := range value.ContextPacks {
		for _, content := range pack.Contents {
			for _, chunk := range content.Chunks {
				live[chunk.Digest] = struct{}{}
			}
		}
	}
	return live
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	s.chunkMu.RLock()
	defer s.chunkMu.RUnlock()
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}
	s.mu.RLock()
	stats := Stats{Consultations: len(s.data.Consultations), ContextPacks: len(s.data.ContextPacks), Results: len(s.data.Results)}
	s.mu.RUnlock()
	if info, err := os.Stat(s.path); err == nil {
		stats.StateBytes = info.Size()
	} else if !os.IsNotExist(err) {
		return Stats{}, err
	}
	chunks, bytesTotal, err := s.chunks.stats()
	if err != nil {
		return Stats{}, err
	}
	stats.Chunks, stats.ChunkBytes = chunks, bytesTotal
	return stats, nil
}
