package billing_test

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/accounting"
)

func TestDuplicateUsageCaptureIsIdempotent(t *testing.T) {
	ctx := context.Background()
	money := accounting.NewMemoryMoneyClient(1_000)
	service := accounting.NewService(money)
	quote := accounting.Quote{
		ID:                 "quote_1",
		Model:              "code-deep",
		MaximumChargeMinor: 100,
		ExpiresAt:          time.Now().Add(time.Minute),
	}

	authorization, err := service.Authorize(ctx, quote)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	usage := accounting.Usage{
		RequestID:         "request_1",
		AttemptID:         "attempt_1",
		ProviderCostMinor: 30,
	}
	if err := service.Settle(ctx, authorization, usage); err != nil {
		t.Fatalf("first settlement: %v", err)
	}
	if err := service.Settle(ctx, authorization, usage); err != nil {
		t.Fatalf("duplicate settlement: %v", err)
	}

	stored, ok := money.Authorization(authorization.ID)
	if !ok {
		t.Fatal("authorization disappeared")
	}
	if stored.CapturedMinor != 30 || stored.HeldMinor != 0 || stored.Status != "settled" {
		t.Fatalf("unexpected authorization after duplicate settlement: %+v", stored)
	}
	if balance := money.Balance(); balance != 970 {
		t.Fatalf("duplicate capture changed balance: got %d, want 970", balance)
	}
}

func TestAdjustmentNeverMutatesOriginalCapture(t *testing.T) {
	ctx := context.Background()
	money := accounting.NewMemoryMoneyClient(1_000)
	service := accounting.NewService(money)
	quote := accounting.Quote{
		ID:                 "quote_2",
		Model:              "code-deep",
		MaximumChargeMinor: 100,
		ExpiresAt:          time.Now().Add(time.Minute),
	}
	authorization, err := service.Authorize(ctx, quote)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := service.Settle(ctx, authorization, accounting.Usage{
		RequestID:         "request_2",
		AttemptID:         "attempt_1",
		ProviderCostMinor: 40,
	}); err != nil {
		t.Fatalf("settle: %v", err)
	}

	before, _ := money.Authorization(authorization.ID)
	if err := money.Adjust(ctx, "adjustment_1", 15, "provider reconciliation credit"); err != nil {
		t.Fatalf("adjust: %v", err)
	}
	if err := money.Adjust(ctx, "adjustment_1", 15, "duplicate delivery"); err != nil {
		t.Fatalf("duplicate adjustment: %v", err)
	}
	after, _ := money.Authorization(authorization.ID)

	if before != after {
		t.Fatalf("append-only adjustment mutated original authorization: before=%+v after=%+v", before, after)
	}
	if amount, ok := money.Adjustment("adjustment_1"); !ok || amount != 15 {
		t.Fatalf("adjustment not recorded once: amount=%d exists=%v", amount, ok)
	}
	if balance := money.Balance(); balance != 975 {
		t.Fatalf("duplicate adjustment changed balance: got %d, want 975", balance)
	}
}
