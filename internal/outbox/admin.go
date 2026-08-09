package outbox

import (
	"context"
	"time"
)

type DeliveryStatus struct {
	Pending           int64      `json:"pending"`
	Locked            int64      `json:"locked"`
	Dead              int64      `json:"dead"`
	PublishedLastHour int64      `json:"publishedLastHour"`
	OldestPendingAt   *time.Time `json:"oldestPendingAt,omitempty"`
}

type DeadLetterRecord struct {
	ID          string    `json:"id"`
	Topic       string    `json:"topic"`
	AggregateID string    `json:"aggregateId"`
	Attempts    int       `json:"attempts"`
	LastError   string    `json:"lastError"`
	DeadAt      time.Time `json:"deadAt"`
	CreatedAt   time.Time `json:"createdAt"`
}

type AdminRepository interface {
	Status(context.Context, time.Time) (DeliveryStatus, error)
	ListDeadLetters(context.Context, int) ([]DeadLetterRecord, error)
	RequeueDeadLetter(context.Context, string, time.Time, bool) error
	PurgePublishedBefore(context.Context, time.Time, int) (int64, error)
}
