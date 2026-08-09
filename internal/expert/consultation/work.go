package consultation

import (
	"context"
	"time"
)

type ClaimCommand struct {
	Route       Route
	WorkerID    string
	LeaseTTL    time.Duration
	ClaimedAt   time.Time
	MaxAttempts int
}

// WorkRepository is an optional stronger contract for durable asynchronous
// execution. Repository remains usable by synchronous desktop flows, while a
// broker worker requires this interface to recover abandoned running jobs.
type WorkRepository interface {
	Claim(context.Context, ClaimCommand) (Consultation, error)
	RenewClaim(context.Context, string, string, time.Duration) (Consultation, error)
	RetryClaim(context.Context, string, string, time.Time, string, int) (Consultation, error)
	RecoverStaleClaims(context.Context, time.Time, int) (int, error)
}

// ClaimResultAttacher fences result attachment with the active worker lease.
// It prevents a slow or partitioned worker from completing work after its
// lease was recovered by another process.
type ClaimResultAttacher interface {
	AttachClaimResult(context.Context, string, string, string) (Consultation, error)
}
