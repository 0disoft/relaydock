package outbox

import (
	"encoding/json"
	"time"
)

type Event struct {
	ID          string
	Topic       string
	AggregateID string
	Payload     json.RawMessage
	AvailableAt time.Time
	Attempts    int
}

type Repository interface {
	Enqueue(Event) error
}
