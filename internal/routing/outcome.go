package routing

import (
	"context"
	"time"
)

// Outcome describes the observable result of one provider attempt. Candidate
// sources may use it to implement process-local health scoring or publish it to
// a shared control plane. Client cancellation should not be treated as a
// provider failure.
type Outcome struct {
	Candidate   Candidate
	StartedAt   time.Time
	CompletedAt time.Time
	Committed   bool
	Success     bool
	ErrorCode   string
	RetryAfter  time.Duration
}

func (o Outcome) Latency() time.Duration {
	if o.StartedAt.IsZero() || o.CompletedAt.IsZero() || o.CompletedAt.Before(o.StartedAt) {
		return 0
	}
	return o.CompletedAt.Sub(o.StartedAt)
}

type OutcomeRecorder interface {
	RecordOutcome(context.Context, Outcome) error
}
