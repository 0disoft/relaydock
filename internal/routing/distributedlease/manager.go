package distributedlease

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/idgen"
	"github.com/0disoft/relaydock/internal/routing"
)

const (
	acquireResultAtCapacity int64 = -1
	acquireResultCollision  int64 = -2
)

// ScriptExecutor executes an atomic integer-returning server-side script.
// The abstraction keeps the routing domain independent from a specific Valkey
// client while preserving atomic lease semantics across gateway replicas.
type ScriptExecutor interface {
	ExecuteInt64(context.Context, string, []string, []string) (int64, error)
}

// Manager implements routing.LeaseManager with one sorted set per provider
// account. Expired members are pruned inside the same script that acquires or
// renews a lease, so no gateway process can oversubscribe an account merely by
// racing another process.
type Manager struct {
	executor ScriptExecutor
	prefix   string
	grace    time.Duration
	newID    func(string) string
}

func New(executor ScriptExecutor, prefix string) (*Manager, error) {
	if executor == nil {
		return nil, fmt.Errorf("%w: distributed lease script executor", core.ErrInvalidConfiguration)
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "arg"
	}
	if len(prefix) > 96 || strings.ContainsAny(prefix, "\r\n\x00{}") {
		return nil, fmt.Errorf("%w: invalid distributed lease key prefix", core.ErrInvalidConfiguration)
	}
	return &Manager{
		executor: executor,
		prefix:   prefix,
		grace:    time.Minute,
		newID:    idgen.New,
	}, nil
}

func (m *Manager) Acquire(ctx context.Context, candidate routing.Candidate, ttl time.Duration) (routing.Lease, error) {
	if err := ctx.Err(); err != nil {
		return routing.Lease{}, err
	}
	if err := validateTTL(ttl); err != nil {
		return routing.Lease{}, err
	}
	accountID := strings.TrimSpace(candidate.AccountID)
	if accountID == "" {
		accountID = strings.TrimSpace(candidate.Provider) + "/default"
	}
	if accountID == "/default" {
		return routing.Lease{}, fmt.Errorf("%w: provider account identity", core.ErrInvalidArgument)
	}
	limit := candidate.ConcurrencyLimit
	if limit <= 0 {
		limit = 1
	}
	leaseID := m.newID("lease")
	expiresAtMilliseconds, err := m.executor.ExecuteInt64(ctx, AcquireScript, []string{m.key(accountID)}, []string{
		strconv.FormatInt(ttl.Milliseconds(), 10),
		strconv.Itoa(limit),
		leaseID,
		strconv.FormatInt(m.grace.Milliseconds(), 10),
	})
	if err != nil {
		return routing.Lease{}, fmt.Errorf("acquire distributed provider lease: %w", err)
	}
	switch expiresAtMilliseconds {
	case acquireResultAtCapacity:
		return routing.Lease{}, core.ErrRateLimited
	case acquireResultCollision:
		return routing.Lease{}, fmt.Errorf("%w: generated lease identifier collision", core.ErrConflict)
	default:
		if expiresAtMilliseconds <= 0 {
			return routing.Lease{}, fmt.Errorf("%w: invalid lease acquisition result %d", core.ErrInvalidConfiguration, expiresAtMilliseconds)
		}
	}
	return routing.Lease{
		ID:        leaseID,
		AccountID: accountID,
		ExpiresAt: time.UnixMilli(expiresAtMilliseconds).UTC(),
	}, nil
}

func (m *Manager) Renew(ctx context.Context, lease routing.Lease, ttl time.Duration) (routing.Lease, error) {
	if err := ctx.Err(); err != nil {
		return routing.Lease{}, err
	}
	if err := validateTTL(ttl); err != nil {
		return routing.Lease{}, err
	}
	if strings.TrimSpace(lease.ID) == "" || strings.TrimSpace(lease.AccountID) == "" {
		return routing.Lease{}, fmt.Errorf("%w: lease identity", core.ErrInvalidArgument)
	}
	expiresAtMilliseconds, err := m.executor.ExecuteInt64(ctx, RenewScript, []string{m.key(lease.AccountID)}, []string{
		strconv.FormatInt(ttl.Milliseconds(), 10),
		lease.ID,
		strconv.FormatInt(m.grace.Milliseconds(), 10),
	})
	if err != nil {
		return routing.Lease{}, fmt.Errorf("renew distributed provider lease: %w", err)
	}
	if expiresAtMilliseconds == 0 {
		return routing.Lease{}, core.ErrLeaseLost
	}
	if expiresAtMilliseconds < 0 {
		return routing.Lease{}, fmt.Errorf("%w: invalid lease renewal result %d", core.ErrInvalidConfiguration, expiresAtMilliseconds)
	}
	lease.ExpiresAt = time.UnixMilli(expiresAtMilliseconds).UTC()
	return lease, nil
}

func (m *Manager) Release(ctx context.Context, lease routing.Lease) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(lease.ID) == "" || strings.TrimSpace(lease.AccountID) == "" {
		return nil
	}
	if _, err := m.executor.ExecuteInt64(ctx, ReleaseScript, []string{m.key(lease.AccountID)}, []string{
		lease.ID,
		strconv.FormatInt(m.grace.Milliseconds(), 10),
	}); err != nil {
		return fmt.Errorf("release distributed provider lease: %w", err)
	}
	return nil
}

func (m *Manager) key(accountID string) string {
	digest := sha256.Sum256([]byte(accountID))
	// The hash tag keeps every script on one cluster slot while avoiding raw
	// account identifiers in infrastructure keys and logs.
	return m.prefix + ":provider-leases:{" + hex.EncodeToString(digest[:16]) + "}"
}

func validateTTL(ttl time.Duration) error {
	if ttl < time.Millisecond || ttl > 24*time.Hour {
		return fmt.Errorf("%w: provider lease TTL must be between 1ms and 24h", core.ErrInvalidArgument)
	}
	return nil
}
