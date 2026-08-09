# 22. Runtime Journal and Transactional Outbox

## Purpose

Record Gateway requests, provider retries, final usage, and external settlement events as one traceable lifecycle. Requests and attempts remain separate because recording only the final successful provider call loses cost and failure evidence from earlier attempts.

## Data Model

```text
runtime.requests
  1 -- N runtime.provider_attempts
  1 -- N runtime.usage_events
  1 -- N outbox.events
```

`runtime.requests` represents one user request. Create a separate `provider_attempts` row for every retry or fallback. Attribute usage to provider attempts and preserve the total across attempts on the completed request.

## Lifecycle

```text
BeginRequest
  -> BeginAttempt
  -> optional MarkCommitted
  -> FinishAttempt
  -> zero or more retry attempts
  -> FinishRequest + InsertOutboxEvent in one transaction
```

Record delivery of the first semantic event in `first_semantic_event_at`. After this boundary, do not hide failures by retrying another provider; retain the committed attempt.

## Idempotency

- Identical writes for the same request or attempt ID may succeed idempotently.
- The same ID with different tenant, model, usage, state, or payload is a conflict.
- The outbox event ID is the downstream idempotency key.
- Reject reinsertion of a different payload under the same event ID as corruption.

## Transaction Boundary

Run `FinishRequest` and request-completed outbox insertion in the same PostgreSQL transaction. Publishing separately after commit can permanently lose settlement events on process crash and is prohibited.

## Outbox Worker

`outboxd` claims batches with `FOR UPDATE SKIP LOCKED` and stores:

```text
locked_by
locked_at
lock_expires_at
attempts
last_error
published_at or dead_at
```

Completion, retry, dead-letter, and release validate both worker identity and completion before lease expiry, fencing stale workers from recording late success.

## Webhook Delivery

- Send the exact event-envelope JSON body.
- Put the event ID in `Idempotency-Key`.
- Sign timestamp and body with HMAC-SHA256.
- Receivers validate the accepted time window, future timestamps, and body integrity.
- Do not follow redirects.
- Reject non-loopback plaintext HTTP without an explicit development option.
- Treat 2xx as success. Treat 409 as idempotent success only with `X-AI-Runtime-Delivery-Status: duplicate` and the same event ID.
- Apply `Retry-After` within a bounded retry schedule.
- Move permanent 4xx responses and exhausted retries to dead letter.

## Management Commands

```powershell
go run ./cmd/outboxctl status
go run ./cmd/outboxctl list --state dead
go run ./cmd/outboxctl requeue --id <eventId>
go run ./cmd/outboxctl purge --published-before 720h
```

Follow `-help` for exact options. Confirm downstream cause and payload compatibility before reprocessing dead letters.

## Reference Consumer

`cmd/webhook-sink` is not a production money ledger. It is a local and CI contract consumer for signature verification, event idempotency, and payload conflicts. It accepts the same ID and body idempotently with 409 plus the event ID and duplicate header, and rejects the same ID with a different body as 422.

## Remaining Money Boundary

The outbox currently delivers trustworthy usage events. A separate money-platform must own customer Mandarin balances, quotes, holds, captures, releases, and refunds. Never edit Gateway usage values as if they were customer balances.
