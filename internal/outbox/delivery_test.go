package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

type publisherFunc func(context.Context, Event) error

func (f publisherFunc) Publish(ctx context.Context, event Event) error { return f(ctx, event) }

func TestWorkerPublishesAndMarksEvent(t *testing.T) {
	repository := NewMemoryDeliveryRepository()
	at := time.Unix(1_700_000_000, 0).UTC()
	if err := repository.Enqueue(Event{ID: "evt-1", Topic: "runtime.request.finished", AggregateID: "req-1", Payload: []byte(`{"ok":true}`), AvailableAt: at}); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.Claim(context.Background(), ClaimCommand{WorkerID: "worker-1", Limit: 1, LeaseTTL: time.Minute, Now: at})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: events=%d err=%v", len(claimed), err)
	}
	worker := newTestWorker(t, repository, publisherFunc(func(context.Context, Event) error { return nil }), at)
	worker.publishOne(context.Background(), claimed[0])

	record := repository.Snapshot()[0]
	if record.PublishedAt.IsZero() || record.LockedBy != "" || record.Attempts != 0 {
		t.Fatalf("unexpected published record: %+v", record)
	}
}

func TestWorkerRetriesWithRetryAfter(t *testing.T) {
	repository := NewMemoryDeliveryRepository()
	at := time.Unix(1_700_000_000, 0).UTC()
	if err := repository.Enqueue(Event{ID: "evt-1", Topic: "runtime.request.finished", AggregateID: "req-1", Payload: []byte(`{"ok":true}`), AvailableAt: at}); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.Claim(context.Background(), ClaimCommand{WorkerID: "worker-1", Limit: 1, LeaseTTL: time.Minute, Now: at})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: events=%d err=%v", len(claimed), err)
	}
	worker := newTestWorker(t, repository, publisherFunc(func(context.Context, Event) error {
		return RetryAfterError{Err: errors.New("rate limited"), After: 37 * time.Second}
	}), at)
	worker.publishOne(context.Background(), claimed[0])

	record := repository.Snapshot()[0]
	if record.Attempts != 1 || !record.AvailableAt.Equal(at.Add(37*time.Second)) || record.LockedBy != "" {
		t.Fatalf("unexpected retry record: %+v", record)
	}
}

func TestWorkerDeadLettersPermanentFailure(t *testing.T) {
	repository := NewMemoryDeliveryRepository()
	at := time.Unix(1_700_000_000, 0).UTC()
	if err := repository.Enqueue(Event{ID: "evt-1", Topic: "runtime.request.finished", AggregateID: "req-1", Payload: []byte(`{"ok":true}`), AvailableAt: at}); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.Claim(context.Background(), ClaimCommand{WorkerID: "worker-1", Limit: 1, LeaseTTL: time.Minute, Now: at})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: events=%d err=%v", len(claimed), err)
	}
	worker := newTestWorker(t, repository, publisherFunc(func(context.Context, Event) error {
		return PermanentError{Err: errors.New("invalid webhook contract")}
	}), at)
	worker.publishOne(context.Background(), claimed[0])

	record := repository.Snapshot()[0]
	if record.DeadAt.IsZero() || record.Attempts != 1 || record.LastError != "invalid webhook contract" {
		t.Fatalf("unexpected dead-letter record: %+v", record)
	}
}

func TestMemoryDeliveryRepositoryFencesStaleWorker(t *testing.T) {
	repository := NewMemoryDeliveryRepository()
	at := time.Unix(1_700_000_000, 0).UTC()
	if err := repository.Enqueue(Event{ID: "evt-1", Topic: "topic", AggregateID: "aggregate", Payload: []byte(`{}`), AvailableAt: at}); err != nil {
		t.Fatal(err)
	}
	first, err := repository.Claim(context.Background(), ClaimCommand{WorkerID: "worker-old", Limit: 1, LeaseTTL: time.Minute, Now: at})
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim: events=%d err=%v", len(first), err)
	}
	beforeExpiry, err := repository.Claim(context.Background(), ClaimCommand{WorkerID: "worker-new", Limit: 1, LeaseTTL: time.Minute, Now: at.Add(30 * time.Second)})
	if err != nil || len(beforeExpiry) != 0 {
		t.Fatalf("claim before expiry: events=%d err=%v", len(beforeExpiry), err)
	}
	afterExpiry, err := repository.Claim(context.Background(), ClaimCommand{WorkerID: "worker-new", Limit: 1, LeaseTTL: time.Minute, Now: at.Add(61 * time.Second)})
	if err != nil || len(afterExpiry) != 1 {
		t.Fatalf("claim after expiry: events=%d err=%v", len(afterExpiry), err)
	}
	if err := repository.MarkPublished(context.Background(), "evt-1", "worker-old", at.Add(62*time.Second)); !errors.Is(err, core.ErrLeaseLost) {
		t.Fatalf("stale worker was not fenced: %v", err)
	}
	if err := repository.MarkPublished(context.Background(), "evt-1", "worker-new", at.Add(62*time.Second)); err != nil {
		t.Fatalf("new worker completion: %v", err)
	}
}

func TestMemoryDeliveryRepositoryRejectsCompletionAfterLeaseExpiryWithoutReclaim(t *testing.T) {
	repository := NewMemoryDeliveryRepository()
	at := time.Unix(1_700_000_000, 0).UTC()
	if err := repository.Enqueue(Event{ID: "evt-expired", Topic: "topic", AggregateID: "aggregate", Payload: []byte(`{}`), AvailableAt: at}); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.Claim(context.Background(), ClaimCommand{WorkerID: "worker-old", Limit: 1, LeaseTTL: time.Minute, Now: at})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: events=%d err=%v", len(claimed), err)
	}
	if err := repository.MarkPublished(context.Background(), "evt-expired", "worker-old", at.Add(time.Minute)); !errors.Is(err, core.ErrLeaseLost) {
		t.Fatalf("expired lease completed event: %v", err)
	}
}

func TestWorkerRejectsLeaseShorterThanPublishBoundary(t *testing.T) {
	_, err := NewWorker(NewMemoryDeliveryRepository(), publisherFunc(func(context.Context, Event) error { return nil }), WorkerConfig{
		WorkerID:          "worker",
		LeaseTTL:          35 * time.Second,
		PublishTimeout:    30 * time.Second,
		RepositoryTimeout: 10 * time.Second,
	})
	if !errors.Is(err, core.ErrInvalidConfiguration) {
		t.Fatalf("expected invalid configuration, got %v", err)
	}
}

func TestDeterministicJitterIsStableAndBounded(t *testing.T) {
	first := deterministicJitter(10*time.Second, 0.2, "evt-1", 3)
	second := deterministicJitter(10*time.Second, 0.2, "evt-1", 3)
	if first != second {
		t.Fatalf("jitter changed: %s vs %s", first, second)
	}
	if first < 8*time.Second || first > 12*time.Second {
		t.Fatalf("jitter out of bounds: %s", first)
	}
}

func newTestWorker(t *testing.T, repository DurableRepository, publisher Publisher, now time.Time) *Worker {
	t.Helper()
	worker, err := NewWorker(repository, publisher, WorkerConfig{
		WorkerID:          "worker-1",
		LeaseTTL:          time.Minute,
		PublishTimeout:    time.Second,
		RepositoryTimeout: time.Second,
		RetryBase:         time.Second,
		RetryMaximum:      time.Minute,
		JitterRatio:       0,
		Now:               func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	// Zero is interpreted as the production default, so force deterministic
	// retry scheduling for unit tests after construction.
	worker.config.JitterRatio = 0
	return worker
}
