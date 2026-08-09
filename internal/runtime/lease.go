package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/routing"
)

func (g *Gateway) validate() error {
	if g == nil || g.Compiler == nil || g.Router == nil || g.Providers == nil || g.Candidates == nil {
		return fmt.Errorf("%w: gateway dependencies", core.ErrInvalidConfiguration)
	}
	return nil
}

func (g *Gateway) acquireLease(ctx context.Context, candidate routing.Candidate) (routing.Lease, error) {
	if g.Leases == nil {
		return routing.Lease{}, nil
	}
	ttl := g.LeaseTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return g.Leases.Acquire(ctx, candidate, ttl)
}

func (g *Gateway) releaseLease(ctx context.Context, lease routing.Lease) error {
	if g.Leases == nil || lease.ID == "" {
		return nil
	}
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := g.Leases.Release(releaseCtx, lease); err != nil {
		return fmt.Errorf("release provider lease: %w", err)
	}
	return nil
}

func (g *Gateway) startLeaseRenewal(
	ctx context.Context,
	cancel context.CancelFunc,
	lease routing.Lease,
	errorsOut chan<- error,
) func() {
	if g.Leases == nil || lease.ID == "" {
		return func() {}
	}
	ttl := g.LeaseTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	interval := ttl / 3
	if interval < time.Second {
		interval = time.Second
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		current := lease
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				renewCtx, renewCancel := context.WithTimeout(context.WithoutCancel(ctx), minDuration(ttl/2, 10*time.Second))
				renewed, err := g.Leases.Renew(renewCtx, current, ttl)
				renewCancel()
				if err != nil {
					select {
					case errorsOut <- fmt.Errorf("renew provider lease: %w", err):
					default:
					}
					cancel()
					return
				}
				current = renewed
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}
