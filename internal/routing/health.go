package routing

import (
	"context"
	"time"
)

type HealthSample struct {
	Provider   string
	AccountID  string
	Model      string
	Healthy    bool
	TTFT       time.Duration
	ErrorCode  string
	MeasuredAt time.Time
}

type HealthStore interface {
	Record(context.Context, HealthSample) error
	Candidates(context.Context, string) ([]Candidate, error)
}
