# Implementation Status

## Current Stage

`0.5.11-dev` is a reference implementation with operational paths, bounded-file contracts, a deny-by-default Control Plane access policy with static and OIDC identity sources, operating-system-backed desktop provider keys on Windows, macOS, and Linux, and Google Cloud workload-identity resolution for Gateway provider keys, the virtual-key pepper, and the Control signing private key. Without external infrastructure it runs with `local/echo`, an atomic JSON store, and memory leases. PostgreSQL and Valkey enable durable consultations, virtual keys, signed Control snapshots, the runtime journal, transactional outbox, and distributed provider leases.

This is not a claim of full production readiness. Real provider accounts, the money-platform, a complete Go 1.26 dependency build, live PostgreSQL and Valkey, Wails installers and updates, and code signing still require validation in target environments.

## Component Status

| Area | Status | Implemented | Remaining production work |
|---|---|---|---|
| Canonical protocol | Implemented and tested | OpenAI Responses/Chat, Anthropic Messages, Gemini encode/decode, strict loss reports | More real-provider fixtures and version-drift monitoring |
| Live streaming | Implemented and tested | Immediate deltas, semantic commit, tool deltas, terminal validation, midstream EOF failure | Provider-specific official resume adapters |
| Gateway composition | Implemented and tested | Environment and GCP workload-identity provider credentials, virtual routes, policy-limited direct models, authoritative signed route/price swaps | Large route benchmarks, hot-reload soak, and additional secret-manager adapters |
| Provider adapters | Implemented | OpenAI, Anthropic, Google, DeepSeek, OpenRouter, generic OpenAI-compatible | Real-account conformance and billed-usage reconciliation |
| Errors and retry safety | Implemented and tested | Stable taxonomy, Retry-After, pre-semantic retry only, visible post-semantic failure | More provider-specific fixtures and explicit continuation |
| Routing and cooldown | Implemented and tested | Capability/region/cost/availability filters, deterministic scoring, degradation and recovery | Measured TTFT/TPS and distributed health sharing |
| Concurrency leases | Implemented and tested | Memory and Valkey Lua acquire/renew/release with server time | Real cluster, Sentinel, and partition validation |
| Signed Control snapshots | Implemented and tested | Key-ID Ed25519, dual trust, direct/file/workload-identity private-key sources, old-key resign, fetch/watch/verify, atomic swap, LKG, fail-closed expiry | Long watch and real secret-manager rotation soak |
| Control stores | Implemented | Atomic local store; PostgreSQL immutable history, advisory lock, polling watch, HA initialization recovery | Real multi-replica contention and database failover |
| Control access policy | Implemented and tested | Digested static credentials, OIDC discovery/JWKS verification and rotation, explicit claim-to-role mapping, gateway/viewer/publisher/admin roles, tenant/project model filtering, provider-account redaction, denial audit | Browser code flow and sessions, membership persistence, revocation, and Console role management |
| Runtime journal | Implemented and tested | Request/attempt/commit/usage/error lifecycle, retry totals, operator UUID startup validation | Partitioning, retention, and load tuning |
| Transactional outbox | Implemented and tested | Atomic finish/event insert, conflict detection, leased worker, fencing, retry, dead letter, graceful stop | Money-platform end-to-end and receiver-outage soak |
| Signed webhook | Implemented and tested | HMAC, replay window, idempotency, redirect/plaintext guards | mTLS or workload identity |
| ContextPack | Implemented and tested | Selection, path and symlink defense, digests, Git revision, redaction, limits | Symbol index, tokenizer estimates, R2 adapter |
| Local consultation store | Implemented and tested | v3 metadata, immutable 32 KiB chunks, digest verification, atomic create, lifecycle locks, migration, compaction | Large-repository crash, disk-full, and antivirus soak |
| PostgreSQL consultations | Implemented | Tenant/project scope, idempotency, claim, lease, recovery, fencing | Long contention and restore validation |
| Expert worker/API | Implemented and tested | Renew/retry/fencing, OpenAI Responses executor, reasoning settings, result validation | Cost authorization, more executors, live model conformance |
| Web handoff | Implemented | Scoped reads, expiry, result import, user-declared attestation | ChatGPT connector UX and organization policy |
| Remote MCP | Partially tested | Host/origin/body guards, bounded argt2 tokens, retiring keys, tenant/project isolation | OIDC/OAuth and live SDK conformance |
| MCP bridge and local IPC | Implemented and tested | STDIO-to-IPC tools, Unix sockets, Windows Named Pipes, frame limits, reconnect | Real Codex/Claude Code/OpenCode conformance and Windows multi-user ACLs |
| Wails desktop | Implemented | Tray, single instance, close-to-tray, autostart, settings, provider runtime, Windows/macOS/Linux provider-key status/save/replace/delete | Physical credential-store smoke, installers, signing, sleep/resume, updater rollback |
| Virtual keys | Implemented and tested | PostgreSQL HMAC keys, scopes/models, revoke, v2 and legacy parsing, direct or workload-identity pepper loading | Multi-pepper rotation, audit, high-volume cache |
| Migrations and bootstrap | Implemented and tested | Embedded SQL, checksums, advisory locks, transactions, organization/project/key CLIs | Real upgrade matrix, PITR, and persisted membership management |
| Accounting domain | Development contract | Quote, hold, capture, release, adjustment, idempotency | Mandarin money-platform client and reconciliation |
| Storage primitives | Partial | Encrypted in-memory secrets, Windows Credential Manager, macOS Keychain, Linux Secret Service, and read-only Google Cloud Secret Manager adapters for provider keys, virtual-key pepper, and Control signing key, opaque credential targets, expiring objects, durable outbox | Physical-device smoke, remaining server-secret coverage, additional KMS adapters, R2/S3 |
| Updater | Staging implemented | Manifest signature, SHA-256, size, pending artifact | Platform replacement and rollback |
| Control Console | Read-only UI | Health, snapshot, and model routes | Login, organizations, keys, routes, audit, usage |
| Packaging | Implemented and tested | Desktop/MCP bundle, server/ops list, 40 KiB audit, strict chunked manifest, deterministic source ZIP | Native installers, artifact signing, cross-version reproducibility |
| CI contracts | Implemented | Go, Buf, sqlc, frontend, PostgreSQL integration gates | Release signatures and target-environment certification |

## Major Gaps Closed by 0.5.2-dev

1. Enforced a 40 KiB file ceiling with reasoned exceptions in CI and releases.
2. Split the monolithic manifest into a root index and verifiable chunks.
3. Added deterministic external source ZIPs and detection of files changing during generation.
4. Split candidate, ingress, stream-decoder, attempt, and lease responsibilities by file.
5. Moved local ContextPack source from metadata into 32 KiB content-addressed chunks.
6. Added legacy-store migration, immutable-pack conflicts, missing-chunk checks, and orphan collection.
7. Made ContextPack and consultation creation atomic in local and PostgreSQL stores.
8. Added signing key IDs and overlapping old/new public-key trust to Control snapshots.
9. Added argt2 key-ID MCP tokens, retiring HMAC keys, collision checks, and claims limits.
10. Added explicit retirement switches for legacy snapshots and argt1 tokens.
11. Included `expertstorectl` and `releasepack` in release artifacts and documentation.
12. Added regression tests for manifests, archive replacement, store migration/compaction, and key rotation.

## Validated Without External Dependencies

- Canonical protocol round trips, strict loss, and unknown-event preservation
- Parallel tool-call ordering, terminal validation, and semantic retry boundaries
- Provider error taxonomy and cooldown
- Signed-snapshot validation, LKG management, and concurrent Control initialization
- ContextPack redaction, digests, path defense, lifecycle, idempotency, and worker fencing
- Scoped argt2 issuance/verification, key rotation, argt1 migration, and bearer-reuse safeguards
- Request/attempt journal domain contracts and outbox lease/fencing/retry/HMAC verification
- Local durable-state corruption detection, migration, chunk hydration, and compaction
- Virtual-key parsing, migration ordering/checksums, and local IPC round trips
- OIDC issuer/audience/time/signature checks, JWKS rotation, claim mapping, scoped-role rejection, and DNS-address validation
- Update staging, storage contracts, size audit, nonregular-file rejection, strict manifests, and deterministic archives

## Conditions for Production Entry

1. Apply every migration to both a clean database and the previous production schema.
2. Pass PostgreSQL integration on a dedicated database and soak multi-replica Control/outbox contention.
3. Verify fail-closed leases during real Valkey standalone, Sentinel, or cluster failures.
4. Pass non-stream, stream, tool, reasoning, 429, and midstream-EOF fixtures for every real provider.
5. Set reconciliation tolerances for provider-reported usage and invoices.
6. Connect money-platform quote, hold, capture, and release end to end through signed outbox events.
7. Pass physical-device tests for Wails lifecycle, Named Pipe ACLs, installer replacement, and updater rollback.
8. Add OIDC/OAuth or workload identity and key rotation to Remote MCP.
9. Complete physical desktop credential-store smoke tests; extend workload-identity resolution to remaining server secrets and target deployment platforms.
10. Verify stream, lease, and worker recovery during a 24-hour soak and graceful shutdown.
