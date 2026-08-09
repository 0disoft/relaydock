package routing

import (
	"context"
	"time"
)

type Lease struct {
	ID        string
	AccountID string
	ExpiresAt time.Time
}

type LeaseManager interface {
	Acquire(context.Context, Candidate, time.Duration) (Lease, error)
	Renew(context.Context, Lease, time.Duration) (Lease, error)
	Release(context.Context, Lease) error
}
