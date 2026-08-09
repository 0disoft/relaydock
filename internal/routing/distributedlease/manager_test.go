package distributedlease

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/routing"
)

type invocation struct {
	script string
	keys   []string
	args   []string
}

type fakeExecutor struct {
	results []int64
	errors  []error
	calls   []invocation
}

func (f *fakeExecutor) ExecuteInt64(_ context.Context, script string, keys, args []string) (int64, error) {
	f.calls = append(f.calls, invocation{script: script, keys: append([]string(nil), keys...), args: append([]string(nil), args...)})
	index := len(f.calls) - 1
	if index < len(f.errors) && f.errors[index] != nil {
		return 0, f.errors[index]
	}
	if index >= len(f.results) {
		return 0, nil
	}
	return f.results[index], nil
}

func TestAcquireRenewRelease(t *testing.T) {
	executor := &fakeExecutor{results: []int64{1_800_000_000_000, 1_800_000_010_000, 1}}
	manager, err := New(executor, "test")
	if err != nil {
		t.Fatal(err)
	}
	manager.newID = func(string) string { return "lease_fixed" }

	lease, err := manager.Acquire(context.Background(), routing.Candidate{
		Provider:         "openai",
		AccountID:        "openai/account-a",
		ConcurrencyLimit: 7,
	}, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if lease.ID != "lease_fixed" || lease.AccountID != "openai/account-a" {
		t.Fatalf("unexpected lease: %#v", lease)
	}
	if len(executor.calls) != 1 || executor.calls[0].script != AcquireScript {
		t.Fatalf("acquire script was not used: %#v", executor.calls)
	}
	if got := executor.calls[0].args; len(got) != 4 || got[0] != "30000" || got[1] != "7" || got[2] != "lease_fixed" {
		t.Fatalf("unexpected acquire arguments: %#v", got)
	}
	if strings.Contains(executor.calls[0].keys[0], "account-a") {
		t.Fatalf("raw account identifier leaked into key: %q", executor.calls[0].keys[0])
	}

	renewed, err := manager.Renew(context.Background(), lease, 40*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !renewed.ExpiresAt.After(lease.ExpiresAt) {
		t.Fatalf("renewal did not advance expiry: old=%s new=%s", lease.ExpiresAt, renewed.ExpiresAt)
	}
	if executor.calls[1].script != RenewScript || executor.calls[1].args[0] != "40000" {
		t.Fatalf("unexpected renewal call: %#v", executor.calls[1])
	}

	if err := manager.Release(context.Background(), renewed); err != nil {
		t.Fatal(err)
	}
	if executor.calls[2].script != ReleaseScript {
		t.Fatalf("release script was not used: %#v", executor.calls[2])
	}
}

func TestAcquireCapacityAndCollision(t *testing.T) {
	executor := &fakeExecutor{results: []int64{acquireResultAtCapacity, acquireResultCollision}}
	manager, err := New(executor, "test")
	if err != nil {
		t.Fatal(err)
	}
	candidate := routing.Candidate{Provider: "openai", AccountID: "account", ConcurrencyLimit: 1}
	if _, err := manager.Acquire(context.Background(), candidate, time.Minute); !errors.Is(err, core.ErrRateLimited) {
		t.Fatalf("expected rate limit, got %v", err)
	}
	if _, err := manager.Acquire(context.Background(), candidate, time.Minute); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("expected collision conflict, got %v", err)
	}
}

func TestRenewLostLease(t *testing.T) {
	executor := &fakeExecutor{results: []int64{0}}
	manager, err := New(executor, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Renew(context.Background(), routing.Lease{ID: "lease", AccountID: "account"}, time.Minute)
	if !errors.Is(err, core.ErrLeaseLost) {
		t.Fatalf("expected lost lease, got %v", err)
	}
}

func TestExecutorErrorsFailClosed(t *testing.T) {
	sentinel := errors.New("valkey unavailable")
	executor := &fakeExecutor{errors: []error{sentinel}}
	manager, err := New(executor, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Acquire(context.Background(), routing.Candidate{Provider: "openai"}, time.Minute)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected underlying failure, got %v", err)
	}
}

func TestInvalidConfigurationAndTTL(t *testing.T) {
	if _, err := New(nil, "test"); !errors.Is(err, core.ErrInvalidConfiguration) {
		t.Fatalf("expected invalid executor, got %v", err)
	}
	executor := &fakeExecutor{}
	manager, err := New(executor, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Acquire(context.Background(), routing.Candidate{Provider: "openai"}, 0); !errors.Is(err, core.ErrInvalidArgument) {
		t.Fatalf("expected invalid TTL, got %v", err)
	}
}
