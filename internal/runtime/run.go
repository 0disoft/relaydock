package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/idgen"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/stream"
	"github.com/0disoft/relaydock/internal/provider"
	"github.com/0disoft/relaydock/internal/routing"
)

func (g *Gateway) Handle(ctx context.Context, envelope canonical.RequestEnvelope) error {
	_, err := g.Run(ctx, envelope, func(context.Context, stream.Event) error { return nil })
	return err
}

func (g *Gateway) Run(ctx context.Context, envelope canonical.RequestEnvelope, sink EventSink) (RunResult, error) {
	return g.RunWithHooks(ctx, envelope, RunHooks{OnEvent: sink})
}

func (g *Gateway) RunWithHooks(ctx context.Context, envelope canonical.RequestEnvelope, hooks RunHooks) (RunResult, error) {
	if err := g.validate(); err != nil {
		return RunResult{}, err
	}
	if err := envelope.Validate(); err != nil {
		return RunResult{}, err
	}
	if hooks.OnEvent == nil {
		hooks.OnEvent = func(context.Context, stream.Event) error { return nil }
	}
	if hooks.OnCommit == nil {
		hooks.OnCommit = func(context.Context, routing.Decision) error { return nil }
	}
	if hooks.OnAttemptStarted == nil {
		hooks.OnAttemptStarted = func(context.Context, AttemptSummary) error { return nil }
	}
	if hooks.OnAttemptCompleted == nil {
		hooks.OnAttemptCompleted = func(context.Context, AttemptSummary, error) error { return nil }
	}

	candidates, err := g.Candidates.Candidates(ctx, envelope.Model)
	if err != nil {
		return RunResult{}, err
	}
	if len(candidates) == 0 {
		return RunResult{}, core.ErrNoRoute
	}

	attemptLimit := g.MaximumAttempts
	if attemptLimit <= 0 {
		attemptLimit = 1
	}
	result := RunResult{AttemptSummaries: make([]AttemptSummary, 0, attemptLimit)}
	excluded := make(map[string]bool, len(candidates))
	var lastErr error

	for result.Attempts < attemptLimit {
		remaining := remainingCandidates(candidates, excluded)
		if len(remaining) == 0 {
			break
		}
		decision, selectErr := g.Router.Select(ctx, routing.Request{
			Requirements: envelope.Requirements,
			MaximumCost:  0,
		}, remaining)
		if selectErr != nil {
			if lastErr != nil {
				return result, lastErr
			}
			return result, selectErr
		}
		key := candidateKey(decision.Candidate)

		lease, leaseErr := g.acquireLease(ctx, decision.Candidate)
		if leaseErr != nil {
			if errors.Is(leaseErr, core.ErrConflict) || errors.Is(leaseErr, core.ErrRateLimited) {
				excluded[key] = true
				lastErr = leaseErr
				continue
			}
			return result, leaseErr
		}

		result.Attempts++
		summary := AttemptSummary{
			ID:        idgen.New("att"),
			Number:    result.Attempts,
			Decision:  decision,
			StartedAt: g.now(),
		}
		if err := hooks.OnAttemptStarted(ctx, summary); err != nil {
			releaseErr := g.releaseLease(ctx, lease)
			return result, errors.Join(err, releaseErr)
		}

		attemptUsage, committed, attemptErr := g.executeAttempt(ctx, envelope, decision, lease, hooks)
		summary.CompletedAt = g.now()
		summary.Committed = committed
		summary.Usage = attemptUsage
		result.TotalUsage.Add(attemptUsage)
		if attemptErr != nil {
			summary.ErrorCode = provider.ErrorCode(attemptErr)
			summary.Error = attemptErr.Error()
			lastErr = attemptErr
		}
		result.AttemptSummaries = append(result.AttemptSummaries, summary)
		g.recordOutcome(ctx, summary, attemptErr)
		if hookErr := hooks.OnAttemptCompleted(ctx, summary, attemptErr); hookErr != nil {
			return result, hookErr
		}

		if attemptErr == nil {
			result.Decision = decision
			result.Usage = attemptUsage
			return result, nil
		}
		if committed || !provider.IsRetryable(attemptErr) {
			return result, attemptErr
		}

		excluded[key] = true
		if result.Attempts >= attemptLimit {
			break
		}
		if delay := g.retryDelay(result.Attempts, attemptErr); delay > 0 {
			if sleepErr := g.sleep(ctx, delay); sleepErr != nil {
				return result, sleepErr
			}
		}
	}

	if lastErr == nil {
		lastErr = core.ErrNoRoute
	}
	return result, lastErr
}

func (g *Gateway) recordOutcome(ctx context.Context, summary AttemptSummary, attemptErr error) {
	recorder, ok := g.Candidates.(routing.OutcomeRecorder)
	if !ok {
		return
	}
	outcome := routing.Outcome{
		Candidate:   summary.Decision.Candidate,
		StartedAt:   summary.StartedAt,
		CompletedAt: summary.CompletedAt,
		Committed:   summary.Committed,
		Success:     attemptErr == nil,
		ErrorCode:   provider.ErrorCode(attemptErr),
		RetryAfter:  provider.RetryAfter(attemptErr),
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	_ = recorder.RecordOutcome(recordCtx, outcome)
}
