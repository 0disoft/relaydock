package runtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/protocol/stream"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
	"github.com/your-org/ai-runtime-gateway/internal/routing"
)

func (g *Gateway) retryDelay(attempt int, err error) time.Duration {
	if providerDelay := provider.RetryAfter(err); providerDelay > 0 {
		return minDuration(providerDelay, g.retryMaximum())
	}
	base := g.RetryBaseDelay
	if base <= 0 {
		return 0
	}
	delay := base
	for i := 1; i < attempt; i++ {
		if delay >= g.retryMaximum()/2 {
			return g.retryMaximum()
		}
		delay *= 2
	}
	return minDuration(delay, g.retryMaximum())
}

func (g *Gateway) retryMaximum() time.Duration {
	if g.RetryMaximum <= 0 {
		return 2 * time.Second
	}
	return g.RetryMaximum
}

func (g *Gateway) now() time.Time {
	if g.Now == nil {
		return time.Now().UTC()
	}
	return g.Now().UTC()
}

func (g *Gateway) sleep(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	if g.Sleep == nil {
		return sleepContext(ctx, duration)
	}
	return g.Sleep(ctx, duration)
}

func remainingCandidates(candidates []routing.Candidate, excluded map[string]bool) []routing.Candidate {
	remaining := make([]routing.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !excluded[candidateKey(candidate)] {
			remaining = append(remaining, candidate)
		}
	}
	return remaining
}

func candidateKey(candidate routing.Candidate) string {
	return candidate.Provider + "/" + candidate.AccountID + "/" + candidate.Model
}

func isSemanticEvent(kind stream.EventKind) bool {
	switch kind {
	case stream.EventContentDelta, stream.EventToolCallDelta, stream.EventReasoningDelta:
		return true
	default:
		return false
	}
}

func cloneEvent(event stream.Event) stream.Event {
	event.Delta = append([]byte(nil), event.Delta...)
	if event.Usage != nil {
		usage := *event.Usage
		event.Usage = &usage
	}
	if event.Extension != nil {
		extension := make(map[string]json.RawMessage, len(event.Extension))
		for key, value := range event.Extension {
			extension[key] = append([]byte(nil), value...)
		}
		event.Extension = extension
	}
	return event
}

func receiveLeaseError(errorsIn <-chan error) error {
	select {
	case err := <-errorsIn:
		return err
	default:
		return nil
	}
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
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
