package outbox

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
)

type DeliveryRecord struct {
	Event
	LockedBy      string
	LockedAt      time.Time
	LockExpiresAt time.Time
	PublishedAt   time.Time
	DeadAt        time.Time
	LastError     string
}

type MemoryDeliveryRepository struct {
	mu     sync.Mutex
	events map[string]DeliveryRecord
}

func NewMemoryDeliveryRepository() *MemoryDeliveryRepository {
	return &MemoryDeliveryRepository{events: make(map[string]DeliveryRecord)}
}

func (r *MemoryDeliveryRepository) Enqueue(event Event) error {
	event.ID = strings.TrimSpace(event.ID)
	event.Topic = strings.TrimSpace(event.Topic)
	event.AggregateID = strings.TrimSpace(event.AggregateID)
	if event.Topic == "" || event.AggregateID == "" || len(event.Payload) == 0 {
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
	r.events[event.ID] = DeliveryRecord{Event: event}
	return nil
}

func (r *MemoryDeliveryRepository) Claim(ctx context.Context, command ClaimCommand) ([]ClaimedEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command.WorkerID = strings.TrimSpace(command.WorkerID)
	if command.WorkerID == "" || command.Limit <= 0 || command.LeaseTTL <= 0 || command.Now.IsZero() {
		return nil, core.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	available := make([]DeliveryRecord, 0)
	for _, record := range r.events {
		if !record.PublishedAt.IsZero() || !record.DeadAt.IsZero() || record.AvailableAt.After(command.Now) {
			continue
		}
		if record.LockedBy != "" && record.LockExpiresAt.After(command.Now) {
			continue
		}
		available = append(available, cloneDeliveryRecord(record))
	}
	sort.Slice(available, func(i, j int) bool {
		if !available[i].AvailableAt.Equal(available[j].AvailableAt) {
			return available[i].AvailableAt.Before(available[j].AvailableAt)
		}
		return available[i].ID < available[j].ID
	})
	if len(available) > command.Limit {
		available = available[:command.Limit]
	}
	claimed := make([]ClaimedEvent, 0, len(available))
	for _, record := range available {
		record.LockedBy = command.WorkerID
		record.LockedAt = command.Now.UTC()
		record.LockExpiresAt = command.Now.UTC().Add(command.LeaseTTL)
		r.events[record.ID] = record
		claimed = append(claimed, ClaimedEvent{
			Event:         cloneEvent(record.Event),
			LockedBy:      record.LockedBy,
			LockExpiresAt: record.LockExpiresAt,
		})
	}
	return claimed, nil
}

func (r *MemoryDeliveryRepository) MarkPublished(ctx context.Context, eventID, workerID string, publishedAt time.Time) error {
	return r.complete(ctx, eventID, workerID, publishedAt, func(record DeliveryRecord) DeliveryRecord {
		record.PublishedAt = publishedAt.UTC()
		record.LastError = ""
		return clearDeliveryLock(record)
	})
}

func (r *MemoryDeliveryRepository) Retry(ctx context.Context, eventID, workerID string, failedAt, availableAt time.Time, message string) error {
	if availableAt.IsZero() {
		return core.ErrInvalidArgument
	}
	return r.complete(ctx, eventID, workerID, failedAt, func(record DeliveryRecord) DeliveryRecord {
		record.Attempts++
		record.AvailableAt = availableAt.UTC()
		record.LastError = boundedErrorText(message)
		return clearDeliveryLock(record)
	})
}

func (r *MemoryDeliveryRepository) DeadLetter(ctx context.Context, eventID, workerID string, deadAt time.Time, message string) error {
	return r.complete(ctx, eventID, workerID, deadAt, func(record DeliveryRecord) DeliveryRecord {
		record.Attempts++
		record.DeadAt = deadAt.UTC()
		record.LastError = boundedErrorText(message)
		return clearDeliveryLock(record)
	})
}

func (r *MemoryDeliveryRepository) Release(ctx context.Context, eventID, workerID string, releasedAt, availableAt time.Time) error {
	if availableAt.IsZero() {
		return core.ErrInvalidArgument
	}
	return r.complete(ctx, eventID, workerID, releasedAt, func(record DeliveryRecord) DeliveryRecord {
		if record.AvailableAt.After(availableAt) {
			record.AvailableAt = availableAt.UTC()
		}
		return clearDeliveryLock(record)
	})
}

func (r *MemoryDeliveryRepository) Snapshot() []DeliveryRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]DeliveryRecord, 0, len(r.events))
	for _, record := range r.events {
		out = append(out, cloneDeliveryRecord(record))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *MemoryDeliveryRepository) complete(ctx context.Context, eventID, workerID string, completedAt time.Time, update func(DeliveryRecord) DeliveryRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	eventID = strings.TrimSpace(eventID)
	workerID = strings.TrimSpace(workerID)
	if eventID == "" || workerID == "" || completedAt.IsZero() {
		return core.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	record, exists := r.events[eventID]
	if !exists {
		return core.ErrNotFound
	}
	if record.LockedBy != workerID || !record.LockExpiresAt.After(completedAt.UTC()) || !record.PublishedAt.IsZero() || !record.DeadAt.IsZero() {
		return core.ErrLeaseLost
	}
	r.events[eventID] = update(record)
	return nil
}

func clearDeliveryLock(record DeliveryRecord) DeliveryRecord {
	record.LockedBy = ""
	record.LockedAt = time.Time{}
	record.LockExpiresAt = time.Time{}
	return record
}

func cloneDeliveryRecord(record DeliveryRecord) DeliveryRecord {
	record.Event = cloneEvent(record.Event)
	return record
}

func boundedErrorText(value string) string {
	value = strings.TrimSpace(value)
	const maximum = 4096
	if len(value) > maximum {
		return value[:maximum]
	}
	return value
}
