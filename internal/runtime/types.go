package runtime

import (
	"context"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/protocol/compiler"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/stream"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
	"github.com/your-org/ai-runtime-gateway/internal/routing"
)

type CandidateSource interface {
	Candidates(context.Context, string) ([]routing.Candidate, error)
}

type EventSink func(context.Context, stream.Event) error

type CommitSink func(context.Context, routing.Decision) error

type AttemptStartedSink func(context.Context, AttemptSummary) error

type AttemptCompletedSink func(context.Context, AttemptSummary, error) error

// RunHooks separates route commitment from stream delivery. A route is not
// committed until the first semantic event (or a successful empty response)
// is ready to be forwarded. This prevents a failed pre-token attempt from
// leaking response-start events before a transparent retry.
type RunHooks struct {
	OnAttemptStarted   AttemptStartedSink
	OnAttemptCompleted AttemptCompletedSink
	OnCommit           CommitSink
	OnEvent            EventSink
}

type Gateway struct {
	Compiler        compiler.Compiler
	Router          routing.Router
	Providers       *provider.Registry
	Candidates      CandidateSource
	Leases          routing.LeaseManager
	LossMode        compiler.LossMode
	MaximumAttempts int
	LeaseTTL        time.Duration
	RetryBaseDelay  time.Duration
	RetryMaximum    time.Duration

	Now   func() time.Time
	Sleep func(context.Context, time.Duration) error
}

type AttemptSummary struct {
	ID          string
	Number      int
	Decision    routing.Decision
	StartedAt   time.Time
	CompletedAt time.Time
	Committed   bool
	Usage       stream.Usage
	ErrorCode   string
	Error       string
}

type RunResult struct {
	Decision         routing.Decision
	Usage            stream.Usage
	TotalUsage       stream.Usage
	Attempts         int
	AttemptSummaries []AttemptSummary
}

func NewGateway() *Gateway {
	return &Gateway{
		MaximumAttempts: 2,
		LossMode:        compiler.LossModeStrict,
		LeaseTTL:        5 * time.Minute,
		RetryBaseDelay:  100 * time.Millisecond,
		RetryMaximum:    2 * time.Second,
		Now:             func() time.Time { return time.Now().UTC() },
		Sleep:           sleepContext,
	}
}
