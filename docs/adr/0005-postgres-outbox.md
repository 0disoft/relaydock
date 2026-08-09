# ADR-0005: PostgreSQL Transactional Outbox

- Status: Accepted
- Date: 2026-08-06
- Owners: Persistence, Expert Broker

## Context

Consultation transitions, finalized usage, and settlement requests require data changes and event publication to succeed together. Adopting Kafka or NATS first at the initial scale leaves the dual-write problem intact while adding operational failure points.

## Decision

Write domain-row changes and the outbox insertion in one PostgreSQL transaction. Workers use an atomic `FOR UPDATE SKIP LOCKED` claim with a worker ID and lease expiration. Record publication time after success; after failure, update backoff and the next available time.

## Invariants

- Outbox payloads contain no secrets or raw prompts.
- Event IDs are globally unique, and consumers are idempotent.
- Claim and lease renewal occur in one statement or transaction.
- Delete rows only after the retention policy and downstream reconciliation are complete.

## Scale Trigger

Introduce a broker only after database CPU, lock waits, outbox delay, or retained volume exceeds an agreed SLO. PostgreSQL outbox remains the transaction boundary, and a relay publishes to the broker.
