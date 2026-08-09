package accounting

import (
	"context"
	"sync"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/idgen"
)

type StaticCatalog struct{ Prices map[string]Price }

func (c StaticCatalog) Get(ctx context.Context, model, revision string) (Price, error) {
	if err := ctx.Err(); err != nil {
		return Price{}, err
	}
	p, ok := c.Prices[model+"@"+revision]
	if !ok {
		p, ok = c.Prices[model]
	}
	if !ok {
		return Price{}, core.ErrNotFound
	}
	return p, nil
}

type MemoryMoneyClient struct {
	mu          sync.Mutex
	balance     int64
	holds       map[string]Authorization
	captured    map[string]bool
	adjustments map[string]int64
}

func NewMemoryMoneyClient(balance int64) *MemoryMoneyClient {
	return &MemoryMoneyClient{balance: balance, holds: map[string]Authorization{}, captured: map[string]bool{}, adjustments: map[string]int64{}}
}
func (m *MemoryMoneyClient) AuthorizeHold(ctx context.Context, q Quote) (Authorization, error) {
	if err := ctx.Err(); err != nil {
		return Authorization{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if q.MaximumChargeMinor > m.balance {
		return Authorization{}, core.ErrBudgetExceeded
	}
	m.balance -= q.MaximumChargeMinor
	a := Authorization{ID: idgen.New("auth"), QuoteID: q.ID, HeldMinor: q.MaximumChargeMinor, Status: "held", ExpiresAt: q.ExpiresAt}
	m.holds[a.ID] = a
	return a, nil
}
func (m *MemoryMoneyClient) CaptureUsage(ctx context.Context, a Authorization, u Usage) (Authorization, error) {
	if err := ctx.Err(); err != nil {
		return Authorization{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.holds[a.ID]
	if !ok {
		return Authorization{}, core.ErrNotFound
	}
	key := u.RequestID + "/" + u.AttemptID
	if m.captured[key] {
		return stored, nil
	}
	if u.ProviderCostMinor < 0 || u.ProviderCostMinor > stored.HeldMinor {
		return Authorization{}, core.ErrBudgetExceeded
	}
	stored.CapturedMinor += u.ProviderCostMinor
	stored.HeldMinor -= u.ProviderCostMinor
	stored.Status = "captured"
	m.holds[a.ID] = stored
	m.captured[key] = true
	return stored, nil
}

func (m *MemoryMoneyClient) ReleaseHold(ctx context.Context, a Authorization, amount int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.holds[a.ID]
	if !ok {
		return core.ErrNotFound
	}
	if amount < 0 || amount > stored.HeldMinor {
		return core.ErrInvalidArgument
	}
	stored.HeldMinor -= amount
	m.balance += amount
	if stored.HeldMinor == 0 {
		stored.Status = "settled"
	}
	m.holds[a.ID] = stored
	return nil
}
func (m *MemoryMoneyClient) Adjust(ctx context.Context, id string, amount int64, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.adjustments[id]; ok {
		return nil
	}
	m.adjustments[id] = amount
	m.balance += amount
	_ = reason
	return nil
}
func (m *MemoryMoneyClient) Balance() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.balance
}

func (m *MemoryMoneyClient) Authorization(id string) (Authorization, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.holds[id]
	return a, ok
}

func (m *MemoryMoneyClient) Adjustment(id string) (int64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	amount, ok := m.adjustments[id]
	return amount, ok
}

var _ = time.Now
