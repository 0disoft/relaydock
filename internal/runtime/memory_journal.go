package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

type MemoryJournal struct {
	mu       sync.RWMutex
	requests map[string]MemoryJournalRequest
	attempts map[string]MemoryJournalAttempt
}

type MemoryJournalRequest struct {
	Start     JournalRequest
	Committed time.Time
	Result    JournalRequestResult
	Finished  bool
}

type MemoryJournalAttempt struct {
	Start    JournalAttempt
	Result   JournalAttemptResult
	Finished bool
}

func NewMemoryJournal() *MemoryJournal {
	return &MemoryJournal{
		requests: make(map[string]MemoryJournalRequest),
		attempts: make(map[string]MemoryJournalAttempt),
	}
}

func (j *MemoryJournal) BeginRequest(ctx context.Context, value JournalRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateJournalRequest(value); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if existing, ok := j.requests[value.ID]; ok {
		if existing.Start == value {
			return nil
		}
		return core.ErrConflict
	}
	j.requests[value.ID] = MemoryJournalRequest{Start: value}
	return nil
}

func (j *MemoryJournal) MarkCommitted(ctx context.Context, requestID string, committedAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if requestID == "" || committedAt.IsZero() {
		return core.ErrInvalidArgument
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	entry, ok := j.requests[requestID]
	if !ok {
		return core.ErrNotFound
	}
	if entry.Finished {
		return core.ErrInvalidTransition
	}
	committedAt = committedAt.UTC()
	if !entry.Committed.IsZero() {
		if entry.Committed.Equal(committedAt) {
			return nil
		}
		return core.ErrConflict
	}
	entry.Committed = committedAt
	j.requests[requestID] = entry
	return nil
}

func (j *MemoryJournal) BeginAttempt(ctx context.Context, value JournalAttempt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateJournalAttempt(value); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	request, ok := j.requests[value.RequestID]
	if !ok {
		return core.ErrNotFound
	}
	if request.Finished {
		return core.ErrInvalidTransition
	}
	if existing, ok := j.attempts[value.ID]; ok {
		if existing.Start == value {
			return nil
		}
		return core.ErrConflict
	}
	for _, attempt := range j.attempts {
		if attempt.Start.RequestID == value.RequestID && attempt.Start.Number == value.Number {
			return core.ErrConflict
		}
	}
	j.attempts[value.ID] = MemoryJournalAttempt{Start: value}
	return nil
}

func (j *MemoryJournal) FinishAttempt(ctx context.Context, value JournalAttemptResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateAttemptResult(value); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	entry, ok := j.attempts[value.ID]
	if !ok || entry.Start.RequestID != value.RequestID {
		return core.ErrNotFound
	}
	if entry.Finished {
		if entry.Result == value {
			return nil
		}
		return core.ErrConflict
	}
	entry.Result = value
	entry.Finished = true
	j.attempts[value.ID] = entry
	return nil
}

func (j *MemoryJournal) FinishRequest(ctx context.Context, value JournalRequestResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateRequestResult(value); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	entry, ok := j.requests[value.ID]
	if !ok {
		return core.ErrNotFound
	}
	if entry.Finished {
		if entry.Result == value {
			return nil
		}
		return core.ErrConflict
	}
	for _, attempt := range j.attempts {
		if attempt.Start.RequestID == value.ID && !attempt.Finished {
			return fmt.Errorf("%w: unfinished provider attempt %s", core.ErrInvalidTransition, attempt.Start.ID)
		}
	}
	entry.Result = value
	entry.Finished = true
	j.requests[value.ID] = entry
	return nil
}

func (j *MemoryJournal) Snapshot() (map[string]MemoryJournalRequest, map[string]MemoryJournalAttempt) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	requests := make(map[string]MemoryJournalRequest, len(j.requests))
	attempts := make(map[string]MemoryJournalAttempt, len(j.attempts))
	for key, value := range j.requests {
		requests[key] = value
	}
	for key, value := range j.attempts {
		attempts[key] = value
	}
	return requests, attempts
}

func validateJournalRequest(value JournalRequest) error {
	if value.ID == "" || value.IngressProtocol == "" || value.VirtualModel == "" || value.StartedAt.IsZero() {
		return core.ErrInvalidArgument
	}
	return nil
}

func validateJournalAttempt(value JournalAttempt) error {
	if value.ID == "" || value.RequestID == "" || value.Number <= 0 || value.Provider == "" || value.UpstreamModel == "" || value.StartedAt.IsZero() {
		return core.ErrInvalidArgument
	}
	return nil
}

func validateAttemptResult(value JournalAttemptResult) error {
	if value.ID == "" || value.RequestID == "" || value.CompletedAt.IsZero() {
		return core.ErrInvalidArgument
	}
	switch value.State {
	case AttemptStateCompleted, AttemptStateFailed, AttemptStateCancelled:
		return nil
	default:
		return core.ErrInvalidArgument
	}
}

func validateRequestResult(value JournalRequestResult) error {
	if value.ID == "" || value.CompletedAt.IsZero() || value.Attempts < 0 {
		return core.ErrInvalidArgument
	}
	switch value.State {
	case RequestStateCompleted, RequestStateFailed, RequestStateCancelled:
		return nil
	default:
		return core.ErrInvalidArgument
	}
}
