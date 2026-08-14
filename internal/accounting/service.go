package accounting

/* llmnav/1 module
id=relaydock.accounting.settlement
role=Quote a pinned price revision, authorize a bounded hold, capture provider usage once, and release the unused balance.
owns=usage quote lifecycle|money authorization boundary|usage settlement
excludes=price catalog persistence|customer ledger storage
search=authorize usage hold|settle provider cost|release remaining balance
invariant=Captured provider cost cannot exceed the authorized hold.
invariant=Every quote identifies the price revision used to calculate its maximum charge.
stability=contract
*/

import (
	"context"
	"fmt"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/idgen"
)

type PriceCatalog interface {
	Get(context.Context, string, string) (Price, error)
}
type Service struct {
	money            MoneyClient
	catalog          PriceCatalog
	defaultMaxTokens int64
	quoteTTL         time.Duration
}

func NewService(money MoneyClient) *Service {
	return &Service{money: money, defaultMaxTokens: 128000, quoteTTL: 10 * time.Minute}
}
func NewServiceWithCatalog(money MoneyClient, catalog PriceCatalog) *Service {
	s := NewService(money)
	s.catalog = catalog
	return s
}
func (s *Service) Quote(ctx context.Context, model, revision string) (Quote, error) {
	if err := ctx.Err(); err != nil {
		return Quote{}, err
	}
	if s.catalog == nil {
		return Quote{}, fmt.Errorf("%w: price catalog", core.ErrInvalidConfiguration)
	}
	price, err := s.catalog.Get(ctx, model, revision)
	if err != nil {
		return Quote{}, err
	}
	maximum := ceilDiv(s.defaultMaxTokens*max64(price.OutputPerMillionMinor, price.ReasoningPerMillionMinor), 1_000_000)
	return Quote{ID: idgen.New("quote"), PriceRevisionID: price.RevisionID, Model: model, MaximumChargeMinor: maximum, ExpiresAt: time.Now().UTC().Add(s.quoteTTL)}, nil
}
func (s *Service) Authorize(ctx context.Context, q Quote) (Authorization, error) {
	if time.Now().After(q.ExpiresAt) {
		return Authorization{}, core.ErrConflict
	}
	if s.money == nil {
		return Authorization{}, fmt.Errorf("%w: money client", core.ErrInvalidConfiguration)
	}
	return s.money.AuthorizeHold(ctx, q)
}
func (s *Service) Settle(ctx context.Context, a Authorization, u Usage) error {
	if u.ProviderCostMinor < 0 {
		return core.ErrInvalidArgument
	}
	if u.ProviderCostMinor > a.HeldMinor {
		return core.ErrBudgetExceeded
	}
	updated, err := s.money.CaptureUsage(ctx, a, u)
	if err != nil {
		return err
	}
	remaining := updated.HeldMinor
	if remaining > 0 {
		return s.money.ReleaseHold(ctx, a, remaining)
	}
	return nil
}
func ceilDiv(a, b int64) int64 {
	if a <= 0 {
		return 0
	}
	return (a + b - 1) / b
}
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
