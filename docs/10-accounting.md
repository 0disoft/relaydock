# 10. Accounting

## Separate Objects

```text
Request
  +-- ProviderAttempt*
       +-- UsageEvent*

CustomerCharge
  +-- AuthorizationHold
  +-- Capture
  +-- Release
  +-- Adjustment
```

One customer request may contain multiple provider attempts.

## Flow

```text
quote
-> authorize hold
-> provisional usage
-> final usage
-> capture
-> release remainder
-> reconciliation
-> adjustment
```

## Usage Dimensions

- uncached input
- cache write
- cache read
- visible output
- reasoning output
- image input/output
- audio seconds
- tool fees
- web search
- service tier surcharge
- provider-reported total
- locally estimated total

## Invariants

- Capture the same idempotency key only once.
- Pin the price revision when the request begins.
- Never modify an original ledger entry.
- Record provider-invoice differences as adjustments.
- Deleting Valkey data must never change a balance.
