package outbox

/* llmnav/1 module
id=relaydock.outbox.delivery
role=Deliver leased outbox events with bounded concurrency, fenced completion, deterministic jitter, retry limits, and dead-letter handling.
owns=outbox worker lifecycle|delivery retry policy|dead-letter transition
excludes=outbox SQL persistence|event transport implementation
search=deliver outbox events|retry webhook delivery|dead letter worker
invariant=Lease TTL exceeds the combined publish and repository operation timeouts.
invariant=Every completion call includes the worker identity that owns the claim.
stability=contract
*/

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

// ClaimedEvent is an event leased to one worker. Attempts is the number of
// previous failed delivery attempts; a successful first delivery has zero
// previous attempts.
type ClaimedEvent struct {
	Event
	LockedBy      string
	LockExpiresAt time.Time
}

type ClaimCommand struct {
	WorkerID string
	Limit    int
	LeaseTTL time.Duration
	Now      time.Time
}

// DurableRepository owns worker leases. Every completion operation must be
// fenced by workerID so a stale worker cannot complete a claim that has been
// recovered by another worker.
type DurableRepository interface {
	Claim(context.Context, ClaimCommand) ([]ClaimedEvent, error)
	MarkPublished(context.Context, string, string, time.Time) error
	Retry(context.Context, string, string, time.Time, time.Time, string) error
	DeadLetter(context.Context, string, string, time.Time, string) error
	Release(context.Context, string, string, time.Time, time.Time) error
}

type Publisher interface {
	Publish(context.Context, Event) error
}

// PermanentError tells the worker that retrying cannot make the delivery
// succeed, for example an authenticated webhook returning a non-retryable 4xx.
type PermanentError struct{ Err error }

func (e PermanentError) Error() string {
	if e.Err == nil {
		return "permanent outbox publish failure"
	}
	return e.Err.Error()
}

func (e PermanentError) Unwrap() error { return e.Err }

// RetryAfterError preserves an upstream requested delay such as HTTP
// Retry-After. The worker still clamps it to RetryMaximum.
type RetryAfterError struct {
	Err   error
	After time.Duration
}

func (e RetryAfterError) Error() string {
	if e.Err == nil {
		return "retryable outbox publish failure"
	}
	return e.Err.Error()
}

func (e RetryAfterError) Unwrap() error { return e.Err }

type WorkerConfig struct {
	WorkerID          string
	BatchSize         int
	Concurrency       int
	LeaseTTL          time.Duration
	PollInterval      time.Duration
	PublishTimeout    time.Duration
	RepositoryTimeout time.Duration
	MaximumAttempts   int
	RetryBase         time.Duration
	RetryMaximum      time.Duration
	JitterRatio       float64
	Logger            *slog.Logger
	Now               func() time.Time
	Sleep             func(context.Context, time.Duration) error
}

type Worker struct {
	repository DurableRepository
	publisher  Publisher
	config     WorkerConfig
}

func NewWorker(repository DurableRepository, publisher Publisher, config WorkerConfig) (*Worker, error) {
	if repository == nil || publisher == nil {
		return nil, fmt.Errorf("%w: outbox worker dependencies", core.ErrInvalidConfiguration)
	}
	config.WorkerID = strings.TrimSpace(config.WorkerID)
	if config.WorkerID == "" {
		return nil, fmt.Errorf("%w: outbox worker ID", core.ErrInvalidConfiguration)
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 32
	}
	if config.Concurrency <= 0 {
		config.Concurrency = 4
	}
	if config.Concurrency > config.BatchSize {
		config.Concurrency = config.BatchSize
	}
	if config.LeaseTTL <= 0 {
		config.LeaseTTL = time.Minute
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.PublishTimeout <= 0 {
		config.PublishTimeout = 30 * time.Second
	}
	if config.RepositoryTimeout <= 0 {
		config.RepositoryTimeout = 10 * time.Second
	}
	if config.LeaseTTL <= config.PublishTimeout+config.RepositoryTimeout {
		return nil, fmt.Errorf(
			"%w: outbox lease TTL %s must exceed publish timeout %s plus repository timeout %s",
			core.ErrInvalidConfiguration,
			config.LeaseTTL,
			config.PublishTimeout,
			config.RepositoryTimeout,
		)
	}
	if config.MaximumAttempts <= 0 {
		config.MaximumAttempts = 12
	}
	if config.RetryBase <= 0 {
		config.RetryBase = time.Second
	}
	if config.RetryMaximum <= 0 {
		config.RetryMaximum = 15 * time.Minute
	}
	if config.RetryMaximum < config.RetryBase {
		return nil, fmt.Errorf("%w: outbox maximum retry delay is below its base", core.ErrInvalidConfiguration)
	}
	if config.JitterRatio == 0 {
		config.JitterRatio = 0.2
	}
	if config.JitterRatio < 0 || config.JitterRatio > 1 {
		return nil, fmt.Errorf("%w: outbox jitter ratio must be between zero and one", core.ErrInvalidConfiguration)
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	if config.Sleep == nil {
		config.Sleep = sleepContext
	}
	return &Worker{repository: repository, publisher: publisher, config: config}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		claimCtx, cancel := context.WithTimeout(ctx, w.config.RepositoryTimeout)
		events, err := w.repository.Claim(claimCtx, ClaimCommand{
			WorkerID: w.config.WorkerID,
			Limit:    w.config.BatchSize,
			LeaseTTL: w.config.LeaseTTL,
			Now:      w.now(),
		})
		cancel()
		if err != nil {
			if w.config.Logger != nil {
				w.config.Logger.ErrorContext(ctx, "claim outbox events", "error", err)
			}
			if err := w.config.Sleep(ctx, w.config.PollInterval); err != nil {
				return err
			}
			continue
		}
		if len(events) == 0 {
			if err := w.config.Sleep(ctx, w.config.PollInterval); err != nil {
				return err
			}
			continue
		}
		w.publishBatch(ctx, events)
	}
}

func (w *Worker) publishBatch(ctx context.Context, events []ClaimedEvent) {
	jobs := make(chan ClaimedEvent)
	var workers sync.WaitGroup
	for range w.config.Concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for event := range jobs {
				w.publishOne(ctx, event)
			}
		}()
	}
	for index, event := range events {
		select {
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			w.releaseBatch(events[index:], "worker stopped before publish")
			return
		case jobs <- event:
		}
	}
	close(jobs)
	workers.Wait()
}

func (w *Worker) publishOne(ctx context.Context, claimed ClaimedEvent) {
	publishCtx, cancel := context.WithTimeout(ctx, w.config.PublishTimeout)
	err := w.publisher.Publish(publishCtx, claimed.Event)
	cancel()
	now := w.now()

	if err == nil {
		if markErr := w.repositoryOperation(func(operationCtx context.Context) error {
			return w.repository.MarkPublished(operationCtx, claimed.ID, w.config.WorkerID, now)
		}); markErr != nil {
			if w.config.Logger != nil {
				w.config.Logger.ErrorContext(ctx, "mark outbox event published", "eventId", claimed.ID, "error", markErr)
			}
			return
		}
		if w.config.Logger != nil {
			w.config.Logger.InfoContext(ctx, "published outbox event", "eventId", claimed.ID, "topic", claimed.Topic)
		}
		return
	}

	if ctx.Err() != nil {
		if releaseErr := w.release(claimed, now, now, "worker stopped during publish"); releaseErr != nil && w.config.Logger != nil {
			w.config.Logger.ErrorContext(ctx, "release cancelled outbox event", "eventId", claimed.ID, "error", releaseErr)
		}
		return
	}

	message := boundedError(err)
	var permanent PermanentError
	if errors.As(err, &permanent) || claimed.Attempts+1 >= w.config.MaximumAttempts {
		if deadErr := w.repositoryOperation(func(operationCtx context.Context) error {
			return w.repository.DeadLetter(operationCtx, claimed.ID, w.config.WorkerID, now, message)
		}); deadErr != nil && w.config.Logger != nil {
			w.config.Logger.ErrorContext(ctx, "dead-letter outbox event", "eventId", claimed.ID, "error", deadErr)
		}
		return
	}

	delay := w.retryDelay(claimed.Attempts+1, claimed.ID, err)
	if retryErr := w.repositoryOperation(func(operationCtx context.Context) error {
		return w.repository.Retry(operationCtx, claimed.ID, w.config.WorkerID, now, now.Add(delay), message)
	}); retryErr != nil && w.config.Logger != nil {
		w.config.Logger.ErrorContext(ctx, "retry outbox event", "eventId", claimed.ID, "error", retryErr)
	}
}

func (w *Worker) releaseBatch(events []ClaimedEvent, reason string) {
	now := w.now()
	for _, event := range events {
		if err := w.release(event, now, now, reason); err != nil && w.config.Logger != nil {
			w.config.Logger.Error("release unprocessed outbox event", "eventId", event.ID, "error", err)
		}
	}
}

func (w *Worker) release(event ClaimedEvent, releasedAt, availableAt time.Time, reason string) error {
	err := w.repositoryOperation(func(operationCtx context.Context) error {
		return w.repository.Release(operationCtx, event.ID, w.config.WorkerID, releasedAt, availableAt)
	})
	if err == nil && w.config.Logger != nil {
		w.config.Logger.Debug("released outbox event", "eventId", event.ID, "reason", reason)
	}
	return err
}

func (w *Worker) repositoryOperation(operation func(context.Context) error) error {
	operationCtx, cancel := context.WithTimeout(context.Background(), w.config.RepositoryTimeout)
	defer cancel()
	return operation(operationCtx)
}

func (w *Worker) retryDelay(attempt int, eventID string, err error) time.Duration {
	var retryAfter RetryAfterError
	if errors.As(err, &retryAfter) && retryAfter.After > 0 {
		if retryAfter.After > w.config.RetryMaximum {
			return w.config.RetryMaximum
		}
		return retryAfter.After
	}
	exponent := math.Pow(2, float64(max(attempt-1, 0)))
	delay := time.Duration(float64(w.config.RetryBase) * exponent)
	if delay > w.config.RetryMaximum || delay < 0 {
		delay = w.config.RetryMaximum
	}
	return deterministicJitter(delay, w.config.JitterRatio, eventID, attempt)
}

func deterministicJitter(delay time.Duration, ratio float64, eventID string, attempt int) time.Duration {
	if delay <= 0 || ratio <= 0 {
		return delay
	}
	hasher := fnv.New64a()
	_, _ = fmt.Fprintf(hasher, "%s:%d", eventID, attempt)
	unit := float64(hasher.Sum64()%1_000_001) / 1_000_000
	factor := (1 - ratio) + (2 * ratio * unit)
	jittered := time.Duration(float64(delay) * factor)
	if jittered < 0 {
		return 0
	}
	return jittered
}

func (w *Worker) now() time.Time { return w.config.Now().UTC() }

func boundedError(err error) string {
	if err == nil {
		return ""
	}
	const maximum = 4096
	value := err.Error()
	if len(value) > maximum {
		return value[:maximum] + "…"
	}
	return value
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
