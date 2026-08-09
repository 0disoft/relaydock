package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	expertapp "github.com/your-org/ai-runtime-gateway/internal/expert/app"
	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	"github.com/your-org/ai-runtime-gateway/internal/expert/resultcontract"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
)

type Executor interface {
	Execute(context.Context, consultation.Consultation, contextpack.Pack) (resultcontract.Result, error)
}

// AttestingExecutor exposes provenance for a successful result. Executors
// that cannot prove their runtime keep the explicit unverified marker.
type AttestingExecutor interface {
	ModelAttestation(consultation.Consultation) string
}

type Config struct {
	WorkerID        string
	Route           consultation.Route
	PollInterval    time.Duration
	LeaseTTL        time.Duration
	RenewInterval   time.Duration
	MaximumAttempts int
	Concurrency     int
	RetryBaseDelay  time.Duration
	RetryMaxDelay   time.Duration
	Logger          *slog.Logger
}

type Worker struct {
	app        *expertapp.App
	repository consultation.WorkRepository
	executor   Executor
	config     Config
}

func New(application *expertapp.App, executor Executor, config Config) (*Worker, error) {
	if application == nil || application.Repository == nil || application.Packs == nil || executor == nil {
		return nil, fmt.Errorf("%w: expert worker dependencies", core.ErrInvalidConfiguration)
	}
	repository, ok := application.Repository.(consultation.WorkRepository)
	if !ok {
		return nil, fmt.Errorf("%w: consultation repository does not support leased work", core.ErrInvalidConfiguration)
	}
	if config.WorkerID == "" {
		config.WorkerID = idgen.New("worker")
	}
	if config.Route == "" {
		config.Route = consultation.RouteOpenAIAPIPro
	}
	if config.PollInterval <= 0 {
		config.PollInterval = 500 * time.Millisecond
	}
	if config.LeaseTTL <= 0 {
		config.LeaseTTL = 2 * time.Minute
	}
	if config.RenewInterval <= 0 {
		config.RenewInterval = config.LeaseTTL / 3
	}
	if config.RenewInterval < time.Second || config.RenewInterval >= config.LeaseTTL {
		return nil, fmt.Errorf("%w: invalid worker renewal interval", core.ErrInvalidConfiguration)
	}
	if config.MaximumAttempts <= 0 {
		config.MaximumAttempts = 3
	}
	if config.Concurrency <= 0 {
		config.Concurrency = 1
	}
	if config.Concurrency > 64 {
		return nil, fmt.Errorf("%w: worker concurrency exceeds 64", core.ErrInvalidConfiguration)
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = 2 * time.Second
	}
	if config.RetryMaxDelay <= 0 {
		config.RetryMaxDelay = 2 * time.Minute
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	return &Worker{app: application, repository: repository, executor: executor, config: config}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	if w == nil {
		return fmt.Errorf("%w: nil expert worker", core.ErrInvalidConfiguration)
	}
	if _, err := w.repository.RecoverStaleClaims(ctx, time.Now().UTC(), w.config.MaximumAttempts); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("recover stale consultations: %w", err)
	}

	semaphore := make(chan struct{}, w.config.Concurrency)
	var group sync.WaitGroup
	defer group.Wait()
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}

		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			return nil
		}

		claimed, err := w.repository.Claim(ctx, consultation.ClaimCommand{
			Route:       w.config.Route,
			WorkerID:    w.config.WorkerID,
			LeaseTTL:    w.config.LeaseTTL,
			MaxAttempts: w.config.MaximumAttempts,
		})
		if err != nil {
			<-semaphore
			if errors.Is(err, core.ErrNotFound) {
				timer.Reset(w.config.PollInterval)
				continue
			}
			if errors.Is(err, context.Canceled) {
				return nil
			}
			w.config.Logger.Error("claim expert consultation", "error", err)
			timer.Reset(w.config.PollInterval)
			continue
		}

		group.Add(1)
		go func(item consultation.Consultation) {
			defer group.Done()
			defer func() { <-semaphore }()
			w.process(ctx, item)
		}(claimed)
		timer.Reset(0)
	}
}

func (w *Worker) process(parent context.Context, item consultation.Consultation) {
	executionCtx, cancelExecution := context.WithCancel(parent)
	defer cancelExecution()
	renewCtx, stopRenewal := context.WithCancel(parent)
	renewalErrors := make(chan error, 1)
	renewalDone := make(chan struct{})
	go w.renewLease(renewCtx, cancelExecution, item.ID, renewalErrors, renewalDone)

	pack, err := w.app.Packs.Get(executionCtx, item.ContextPackID)
	var result resultcontract.Result
	if err == nil {
		result, err = w.executor.Execute(executionCtx, item, pack)
	}

	// Stop lease renewal before committing or requeueing. The lease remains
	// valid until LockExpiresAt; stopping only removes the goroutine that could
	// race with completion after the claim has been cleared.
	stopRenewal()
	<-renewalDone
	select {
	case renewalErr := <-renewalErrors:
		if renewalErr != nil {
			w.config.Logger.Warn("expert lease lost; discarding worker outcome", "consultationId", item.ID, "error", renewalErr)
			return
		}
	default:
	}

	if parent.Err() != nil {
		return
	}
	if err == nil {
		attestation := resultcontract.UnverifiedAttestation
		if attesting, ok := w.executor.(AttestingExecutor); ok {
			attestation = attesting.ModelAttestation(item)
		}
		if _, _, commitErr := w.app.StoreClaimResultAttested(parent, item.ID, w.config.WorkerID, attestation, result); commitErr != nil {
			if errors.Is(commitErr, core.ErrLeaseLost) || errors.Is(commitErr, core.ErrConflict) {
				w.config.Logger.Warn("expert result rejected by worker fence", "consultationId", item.ID, "error", commitErr)
				return
			}
			w.config.Logger.Error("commit expert result", "consultationId", item.ID, "error", commitErr)
		}
		return
	}
	if errors.Is(err, context.Canceled) && parent.Err() != nil {
		return
	}

	if provider.IsRetryable(err) || errors.Is(err, context.DeadlineExceeded) {
		delay := w.retryDelay(item.AttemptCount)
		_, retryErr := w.repository.RetryClaim(parent, item.ID, w.config.WorkerID, time.Now().UTC().Add(delay), err.Error(), w.config.MaximumAttempts)
		if retryErr != nil {
			if errors.Is(retryErr, core.ErrLeaseLost) || errors.Is(retryErr, core.ErrConflict) {
				w.config.Logger.Warn("retry rejected by worker fence", "consultationId", item.ID, "error", retryErr, "cause", err)
				return
			}
			w.config.Logger.Error("requeue expert consultation", "consultationId", item.ID, "error", retryErr, "cause", err)
		}
		return
	}
	if _, failureErr := w.app.Repository.SetFailure(parent, item.ID, err.Error()); failureErr != nil {
		w.config.Logger.Error("fail expert consultation", "consultationId", item.ID, "error", failureErr, "cause", err)
	}
}

func (w *Worker) renewLease(ctx context.Context, cancel context.CancelFunc, consultationID string, errorsOut chan<- error, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(w.config.RenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.repository.RenewClaim(ctx, consultationID, w.config.WorkerID, w.config.LeaseTTL); err != nil {
				cancel()
				select {
				case errorsOut <- fmt.Errorf("%w: renew expert worker lease: %v", core.ErrLeaseLost, err):
				default:
				}
				return
			}
		}
	}
}

func (w *Worker) retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := w.config.RetryBaseDelay
	for index := 1; index < attempt && delay < w.config.RetryMaxDelay; index++ {
		if delay > w.config.RetryMaxDelay/2 {
			return w.config.RetryMaxDelay
		}
		delay *= 2
	}
	if delay > w.config.RetryMaxDelay {
		return w.config.RetryMaxDelay
	}
	return delay
}
