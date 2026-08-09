package outbox

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
)

type MemoryRepository struct {
	mu     sync.Mutex
	events map[string]Event
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{events: make(map[string]Event)}
}

func (r *MemoryRepository) Enqueue(event Event) error {
	event.Topic = strings.TrimSpace(event.Topic)
	if event.Topic == "" || len(event.Payload) == 0 {
		return core.ErrInvalidArgument
	}
	if event.ID == "" {
		event.ID = idgen.New("evt")
	}
	if event.AvailableAt.IsZero() {
		event.AvailableAt = time.Now().UTC()
	}
	event.Payload = append([]byte(nil), event.Payload...)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.events[event.ID]; exists {
		return core.ErrConflict
	}
	r.events[event.ID] = event
	return nil
}

// Claim removes and returns currently available events. A production
// repository should claim transactionally with SKIP LOCKED; removing here
// gives deterministic at-most-once ownership to test workers.
func (r *MemoryRepository) Claim(now time.Time, limit int) []Event {
	if limit <= 0 {
		limit = 100
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	available := make([]Event, 0)
	for _, event := range r.events {
		if !event.AvailableAt.After(now) {
			available = append(available, cloneEvent(event))
		}
	}
	sort.Slice(available, func(i, j int) bool {
		if !available[i].AvailableAt.Equal(available[j].AvailableAt) {
			return available[i].AvailableAt.Before(available[j].AvailableAt)
		}
		return available[i].ID < available[j].ID
	})
	if len(available) > limit {
		available = available[:limit]
	}
	for _, event := range available {
		delete(r.events, event.ID)
	}
	return available
}

func (r *MemoryRepository) Requeue(event Event, availableAt time.Time) error {
	event.Attempts++
	event.AvailableAt = availableAt
	return r.Enqueue(event)
}

func (r *MemoryRepository) Snapshot() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Event, 0, len(r.events))
	for _, event := range r.events {
		out = append(out, cloneEvent(event))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func cloneEvent(event Event) Event {
	event.Payload = append([]byte(nil), event.Payload...)
	return event
}
