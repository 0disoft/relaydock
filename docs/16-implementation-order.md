# 16. Implementation Order

This document fixes dependency order and completion criteria rather than listing features. The repository implements the core code for Stages 0, 1, and 2. Remaining work is primarily operational validation against real infrastructure, providers, and desktop environments, plus integration with the money-platform boundary.

## Stage 0A: Contracts -- Complete

Completed scope:

- Canonical request, response, and stream types
- Strict, compatible, and passthrough loss taxonomy
- Provider-error taxonomy
- Consultation transitions and idempotency
- ContextPack manifest and content digest
- MCP tool input/output schemas
- PostgreSQL schemas and embedded migration contract
- Virtual-key v2 format and legacy parser

Completion evidence:

- Protocol, state, and storage tests that run without external modules
- Migration version, checksum, and up/down-pair validation
- Raw preservation of unknown provider events

## Stage 0B: Local Runtime -- Code Complete, Device Validation Pending

Implemented scope:

- Wails v3 tray and single-instance handling
- Close-to-tray and `--background` login autostart
- MCP STDIO bridge to Named Pipe or Unix Domain Socket
- ContextPack preview and consultation creation/read
- Local atomic-state persistence
- Local Gateway start/stop

Operational completion conditions:

- Validate Named Pipe ACLs for standard, administrator, and different Windows user accounts.
- Validate autostart after sleep, logout, login, and updates.
- Validate macOS launch lifecycle and Linux desktop sessions.
- Regression-test MCP stdout contamination, runtime crashes, and reconnection.

## Stage 1A: Gateway Data Plane -- Code Complete

Implemented scope:

- OpenAI Responses and Chat, Anthropic Messages, and Gemini ingress
- OpenAI, Anthropic, Google, DeepSeek, OpenRouter, and OpenAI-compatible adapters
- Virtual-model routes and direct `provider/model` routing
- Live SSE forwarding
- Cancellation, response-size limits, and terminal validation
- Provider-error classification and Retry-After
- Pre-semantic retry and visible post-semantic failure
- Process-local health degradation and cooldown
- Memory and Valkey concurrency leases

Operational completion conditions:

- Pass non-stream, stream, tool, and reasoning conformance with real credentials for every provider.
- Pass 429, exhausted-quota, overloaded, timeout, and transport-reset fixtures.
- Establish tolerances between provider usage and local usage estimates.
- Verify lease limits during Valkey failover.
- Confirm no goroutine, connection, or memory leaks in a 24-hour soak test.

## Stage 1B: Expert Broker -- Code Complete

Implemented scope:

- ContextPack preview, build, and redaction
- Local atomic-file and PostgreSQL repositories
- Consultation create, approve, cancel, list, and get
- Worker claim, renewal, retry, stale recovery, and maximum attempts
- Worker-fenced result commits
- OpenAI Responses expert executor
- ChatGPT web handoff and manual result import
- Result model attestation
- Remote MCP get and submit

Operational completion conditions:

- Test multi-worker contention and crash recovery on real PostgreSQL.
- Reconcile API-model usage with consultation maximum cost.
- Pass live-server conformance for tenant/project isolation in scoped Remote MCP.
- Define OIDC/OAuth and key-ID rotation.
- Provide UI warnings and rereview procedures for stale ContextPack results.

## Stage 1C: Control Snapshot -- Code Complete

Implemented scope:

- Ed25519 signing-key load/create and secret-manager injection
- Local atomic store and PostgreSQL immutable history
- Current, publish, watch, and public-key APIs
- Recovery from multi-replica initialization and update races
- Gateway fetch, watch, signature verification, and atomic route swaps
- Last-known-good persistence and fail-closed snapshot expiry

Operational completion conditions:

- Signing-key rotation and key-ID-based dual trust
- Long Control database failover and watch soak
- A fixed production source of truth between route files and signed Control snapshots
- Final snapshot TTL and recovery objectives

## Stage 2: Managed Operations Foundation -- Core Code Complete

Completed scope:

- Embedded PostgreSQL migration runner
- Organization/project bootstrap CLI
- PostgreSQL virtual-key issue, authenticate, and revoke
- Project scope and model allowlist
- Valkey distributed provider lease
- Runtime request, provider-attempt, and usage journal
- Transactional PostgreSQL outbox
- Worker lease, fencing, dead letter, and signed webhooks
- PostgreSQL integration CI contract

Next work:

- Provider health probes and distributed measurements
- Audit events and administrative RBAC
- Provider-credential KMS and OS-keychain adapters
- Runtime retention and partition policies

## Stage 3: Accounting and Commercialization -- Contract Only

The domain contract and idempotency tests exist for `quote -> hold -> capture -> release/adjustment`. Open real payments and Mandarin deductions only after all of the following:

- `zdp-money-platform` ConnectRPC contract
- Separation of request charges and provider-attempt cost
- Partial-stream failure cost policy
- Late usage adjustments
- Refund, cancellation, and free/paid credit-consumption revisions
- End-to-end verification of the signed PostgreSQL outbox and money-platform consumer idempotency
- Provider-invoice reconciliation

Do not enable payments before usage persistence.

## Stage 4: Release Certification

A release candidate requires all of this evidence:

- Clean-machine build and reproducible artifact hash
- Physical-device records for Wails installer upgrade and rollback
- PostgreSQL backup, PITR, and restore rehearsal
- Valkey failure and provider-outage game day
- Conformance report for each MCP client
- Real provider-bill reconciliation report
- Security review covering SSRF, secret redaction, local IPC ACLs, and token scope
- 24-hour soak and graceful-shutdown report
