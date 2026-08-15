package routing

/* llmnav/1 module
id=relaydock.routing.memory-leases
role=Provide process-local health candidates and expiring account concurrency leases for single-runtime routing.
owns=in-memory candidate health|account lease capacity|lease expiry pruning
search=memory routing leases|account concurrency limit|process local health store
invariant=Candidate slices are copied at the store boundary so callers cannot mutate retained routing state.
invariant=Expired leases are pruned before capacity, renewal, release, or active-count decisions.
risk=concurrency|availability
stability=contract
*/

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/idgen"
)

type MemoryHealthStore struct {
	mu                sync.RWMutex
	candidatesByModel map[string][]Candidate
	samples           []HealthSample
}

func NewMemoryHealthStore() *MemoryHealthStore {
	return &MemoryHealthStore{candidatesByModel: map[string][]Candidate{}}
}
func (s *MemoryHealthStore) SetCandidates(model string, candidates []Candidate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.candidatesByModel[model] = append([]Candidate(nil), candidates...)
}
func (s *MemoryHealthStore) Record(ctx context.Context, sample HealthSample) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = append(s.samples, sample)
	return nil
}
func (s *MemoryHealthStore) Candidates(ctx context.Context, model string) ([]Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Candidate(nil), s.candidatesByModel[model]...), nil
}

type MemoryLeaseManager struct {
	mu        sync.Mutex
	leases    map[string]Lease
	byAccount map[string]map[string]struct{}
}

func NewMemoryLeaseManager() *MemoryLeaseManager {
	return &MemoryLeaseManager{leases: map[string]Lease{}, byAccount: map[string]map[string]struct{}{}}
}
func (m *MemoryLeaseManager) Acquire(ctx context.Context, candidate Candidate, ttl time.Duration) (Lease, error) {
	if err := ctx.Err(); err != nil {
		return Lease{}, err
	}
	if ttl <= 0 {
		return Lease{}, fmt.Errorf("%w: lease TTL", core.ErrInvalidArgument)
	}
	accountID := candidate.AccountID
	if accountID == "" {
		accountID = candidate.Provider + "/default"
	}
	limit := candidate.ConcurrencyLimit
	if limit <= 0 {
		limit = 1
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(time.Now().UTC())
	active := m.byAccount[accountID]
	if len(active) >= limit {
		return Lease{}, core.ErrRateLimited
	}
	lease := Lease{ID: idgen.New("lease"), AccountID: accountID, ExpiresAt: time.Now().UTC().Add(ttl)}
	m.leases[lease.ID] = lease
	if active == nil {
		active = make(map[string]struct{})
		m.byAccount[accountID] = active
	}
	active[lease.ID] = struct{}{}
	return lease, nil
}
func (m *MemoryLeaseManager) Renew(ctx context.Context, lease Lease, ttl time.Duration) (Lease, error) {
	if err := ctx.Err(); err != nil {
		return Lease{}, err
	}
	if ttl <= 0 {
		return Lease{}, core.ErrInvalidArgument
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(time.Now().UTC())
	stored, ok := m.leases[lease.ID]
	if !ok || stored.AccountID != lease.AccountID {
		return Lease{}, core.ErrNotFound
	}
	stored.ExpiresAt = time.Now().UTC().Add(ttl)
	m.leases[lease.ID] = stored
	return stored, nil
}
func (m *MemoryLeaseManager) Release(ctx context.Context, lease Lease) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.leases[lease.ID]
	if !ok {
		return nil
	}
	m.deleteLocked(stored)
	return nil
}
func (m *MemoryLeaseManager) Active(accountID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(time.Now().UTC())
	return len(m.byAccount[accountID])
}
func (m *MemoryLeaseManager) pruneLocked(now time.Time) {
	for _, lease := range m.leases {
		if !lease.ExpiresAt.After(now) {
			m.deleteLocked(lease)
		}
	}
}
func (m *MemoryLeaseManager) deleteLocked(lease Lease) {
	delete(m.leases, lease.ID)
	active := m.byAccount[lease.AccountID]
	delete(active, lease.ID)
	if len(active) == 0 {
		delete(m.byAccount, lease.AccountID)
	}
}
