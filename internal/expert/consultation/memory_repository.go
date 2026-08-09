package consultation

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
)

type MemoryRepository struct {
	mu          sync.RWMutex
	items       map[string]Consultation
	idempotency map[string]idempotencyRecord
	now         func() time.Time
}

type idempotencyRecord struct {
	ConsultationID string
	Fingerprint    string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		items:       map[string]Consultation{},
		idempotency: map[string]idempotencyRecord{},
		now:         func() time.Time { return time.Now().UTC() },
	}
}

func (r *MemoryRepository) Create(ctx context.Context, command CreateCommand) (Consultation, error) {
	if err := ctx.Err(); err != nil {
		return Consultation{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	scope := IdempotencyScope(command)
	fingerprint := RequestFingerprint(command)
	if scope != "" {
		if record, exists := r.idempotency[scope]; exists {
			if record.Fingerprint != fingerprint {
				return Consultation{}, core.ErrConflict
			}
			return r.items[record.ConsultationID], nil
		}
	}
	now := r.nowUTC()
	ttl := command.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	state := StateContextPending
	if strings.TrimSpace(command.ContextPackID) != "" {
		state = StateApprovalPending
	}
	item := Consultation{
		ID:               idgen.New("con"),
		TenantID:         strings.TrimSpace(command.TenantID),
		ProjectID:        strings.TrimSpace(command.ProjectID),
		Objective:        strings.TrimSpace(command.Objective),
		TaskType:         strings.TrimSpace(command.TaskType),
		Route:            command.Route,
		State:            state,
		ContextPackID:    strings.TrimSpace(command.ContextPackID),
		MaximumCostMinor: command.MaximumCostMinor,
		CreatedAt:        now,
		UpdatedAt:        now,
		ExpiresAt:        now.Add(ttl),
	}
	r.items[item.ID] = item
	if scope != "" {
		r.idempotency[scope] = idempotencyRecord{ConsultationID: item.ID, Fingerprint: fingerprint}
	}
	return item, nil
}

func (r *MemoryRepository) Get(ctx context.Context, id string) (Consultation, error) {
	if err := ctx.Err(); err != nil {
		return Consultation{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Consultation{}, core.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item, exists := r.items[id]
	if !exists {
		return Consultation{}, core.ErrNotFound
	}
	if expireConsultation(&item, r.nowUTC()) {
		r.items[id] = item
	}
	return item, nil
}

func (r *MemoryRepository) List(ctx context.Context, limit int) ([]Consultation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	now := r.nowUTC()
	items := make([]Consultation, 0, len(r.items))
	for id, item := range r.items {
		if expireConsultation(&item, now) {
			r.items[id] = item
		}
		items = append(items, item)
	}
	r.mu.Unlock()
	sort.Slice(items, func(left, right int) bool {
		if !items[left].UpdatedAt.Equal(items[right].UpdatedAt) {
			return items[left].UpdatedAt.After(items[right].UpdatedAt)
		}
		return items[left].ID > items[right].ID
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *MemoryRepository) UpdateState(ctx context.Context, id string, expected, next State) (Consultation, error) {
	if err := ctx.Err(); err != nil {
		return Consultation{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item, exists := r.items[id]
	if !exists {
		return Consultation{}, core.ErrNotFound
	}
	if expireConsultation(&item, r.nowUTC()) {
		r.items[id] = item
		return Consultation{}, core.ErrConflict
	}
	if item.State != expected {
		return Consultation{}, core.ErrConflict
	}
	item.State = next
	item.UpdatedAt = r.nowUTC()
	if next != StateRunning {
		ClearClaim(&item)
	}
	r.items[id] = item
	return item, nil
}

func (r *MemoryRepository) AttachContext(ctx context.Context, id, packID string, expected, next State) (Consultation, error) {
	if err := ctx.Err(); err != nil {
		return Consultation{}, err
	}
	packID = strings.TrimSpace(packID)
	if packID == "" {
		return Consultation{}, core.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item, exists := r.items[id]
	if !exists {
		return Consultation{}, core.ErrNotFound
	}
	if item.State != expected {
		return Consultation{}, core.ErrConflict
	}
	item.ContextPackID = packID
	item.State = next
	item.UpdatedAt = r.nowUTC()
	ClearClaim(&item)
	r.items[id] = item
	return item, nil
}

// ClaimNext is retained for synchronous callers. Asynchronous workers should
// use Claim so abandoned work can be recovered by lease expiry.
func (r *MemoryRepository) ClaimNext(ctx context.Context, route string) (Consultation, error) {
	return r.Claim(ctx, ClaimCommand{Route: Route(route), WorkerID: "legacy", LeaseTTL: defaultClaimLease})
}

func (r *MemoryRepository) Claim(ctx context.Context, command ClaimCommand) (Consultation, error) {
	if err := ctx.Err(); err != nil {
		return Consultation{}, err
	}
	normalized, err := NormalizeClaimCommand(command)
	if err != nil {
		return Consultation{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var candidateIDs []string
	for id, item := range r.items {
		if expireConsultation(&item, normalized.ClaimedAt) {
			r.items[id] = item
			continue
		}
		if Claimable(item, normalized) {
			candidateIDs = append(candidateIDs, id)
		}
	}
	sort.Slice(candidateIDs, func(i, j int) bool {
		left, right := r.items[candidateIDs[i]], r.items[candidateIDs[j]]
		if !left.AvailableAt.Equal(right.AvailableAt) {
			if left.AvailableAt.IsZero() {
				return true
			}
			if right.AvailableAt.IsZero() {
				return false
			}
			return left.AvailableAt.Before(right.AvailableAt)
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return left.ID < right.ID
	})
	for _, id := range candidateIDs {
		item := r.items[id]
		if err := ApplyClaim(&item, normalized); err != nil {
			r.items[id] = item
			continue
		}
		r.items[id] = item
		return item, nil
	}
	return Consultation{}, core.ErrNotFound
}

func (r *MemoryRepository) RenewClaim(ctx context.Context, id, workerID string, ttl time.Duration) (Consultation, error) {
	if err := ctx.Err(); err != nil {
		return Consultation{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item, exists := r.items[id]
	if !exists {
		return Consultation{}, core.ErrNotFound
	}
	if err := ApplyRenewClaim(&item, workerID, ttl, r.nowUTC()); err != nil {
		return Consultation{}, err
	}
	r.items[id] = item
	return item, nil
}

func (r *MemoryRepository) RetryClaim(ctx context.Context, id, workerID string, availableAt time.Time, reason string, maximumAttempts int) (Consultation, error) {
	if err := ctx.Err(); err != nil {
		return Consultation{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item, exists := r.items[id]
	if !exists {
		return Consultation{}, core.ErrNotFound
	}
	if err := ApplyRetryClaim(&item, workerID, availableAt, reason, maximumAttempts, r.nowUTC()); err != nil {
		return Consultation{}, err
	}
	r.items[id] = item
	return item, nil
}

func (r *MemoryRepository) RecoverStaleClaims(ctx context.Context, now time.Time, maximumAttempts int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if now.IsZero() {
		now = r.nowUTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	recovered := 0
	for id, item := range r.items {
		if ApplyStaleRecovery(&item, now, maximumAttempts) {
			r.items[id] = item
			recovered++
		}
	}
	return recovered, nil
}

func (r *MemoryRepository) AttachClaimResult(ctx context.Context, id, workerID, resultID string) (Consultation, error) {
	if err := ctx.Err(); err != nil {
		return Consultation{}, err
	}
	resultID = strings.TrimSpace(resultID)
	if resultID == "" {
		return Consultation{}, core.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item, exists := r.items[id]
	if !exists {
		return Consultation{}, core.ErrNotFound
	}
	if err := ValidateClaimOwner(item, workerID, r.nowUTC()); err != nil {
		return Consultation{}, err
	}
	item.ResultID = resultID
	item.State = StateCompleted
	item.UpdatedAt = r.nowUTC()
	ClearClaim(&item)
	r.items[id] = item
	return item, nil
}

func (r *MemoryRepository) AttachResult(ctx context.Context, id, resultID string) (Consultation, error) {
	if err := ctx.Err(); err != nil {
		return Consultation{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item, exists := r.items[id]
	if !exists {
		return Consultation{}, core.ErrNotFound
	}
	if item.State != StateRunning && item.State != StateResultPending {
		return Consultation{}, core.ErrConflict
	}
	item.ResultID = strings.TrimSpace(resultID)
	item.State = StateResultPending
	item.UpdatedAt = r.nowUTC()
	ClearClaim(&item)
	r.items[id] = item
	return item, nil
}

func (r *MemoryRepository) SetFailure(ctx context.Context, id, reason string) (Consultation, error) {
	if err := ctx.Err(); err != nil {
		return Consultation{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item, exists := r.items[id]
	if !exists {
		return Consultation{}, core.ErrNotFound
	}
	if item.Terminal() {
		return Consultation{}, core.ErrConflict
	}
	item.State = StateFailed
	item.FailureReason = boundedFailureReason(reason)
	item.UpdatedAt = r.nowUTC()
	ClearClaim(&item)
	r.items[id] = item
	return item, nil
}

func (r *MemoryRepository) nowUTC() time.Time {
	if r.now == nil {
		return time.Now().UTC()
	}
	return r.now().UTC()
}

func expireConsultation(item *Consultation, now time.Time) bool {
	if item == nil || item.Terminal() || item.ExpiresAt.IsZero() || item.ExpiresAt.After(now) {
		return false
	}
	item.State = StateExpired
	item.UpdatedAt = now.UTC()
	ClearClaim(item)
	return true
}
