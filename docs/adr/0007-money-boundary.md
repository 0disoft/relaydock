# ADR-0007: Money-Platform Boundary

- Status: Accepted
- Date: 2026-08-06
- Owners: Accounting, Money Platform

## Context

The Gateway has the best view of provider token usage and attempts, but it must not own customer balances, top-ups, refunds, expiration, taxes, or the payment ledger. Combining the replaceable model-provider data plane and the money ledger in one database expands failure and migration impact.

## Decision

The Gateway owns requests, provider attempts, measured usage, and price-revision references. The money-platform owns balances, authorization holds, captures, releases, adjustments, and refunds. Do not replicate customer balances into the Gateway database.

The settlement contract follows `quote -> hold -> provisional usage -> capture/release -> adjustment`. Every mutation requires an idempotency key and never modifies original ledger rows.

## Failure Policy

When the money-platform is unavailable, either fail closed for new paid requests or use only a bounded preauthorized lease, according to product policy. Never treat a Valkey balance as truth or report a failed capture as successful.

## Validation

Pass contract tests for duplicate delivery, retry after timeout, provider-usage corrections, partial-stream cost, refunds, and reconciliation.
