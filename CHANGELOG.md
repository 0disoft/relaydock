# Changelog

## 0.5.8-dev -- Linux Secret Service Credential Store

- Added a native Secret Service backend over the per-user D-Bus session bus without invoking `secret-tool` or another shell command.
- Implemented plain-session negotiation, opaque attribute search, default-collection writes, locked item and collection prompts, replacement, read, and idempotent delete behavior.
- Propagated operation contexts through the native-store backend contract and failed closed when D-Bus, Secret Service, or the default collection is unavailable.
- Rejected missing, duplicate, malformed, or invalid-path results and omitted remote D-Bus error bodies from returned errors.
- Promoted the already locked `github.com/godbus/dbus/v5` module to a direct dependency without changing its version or downloading a new package.
- Added deterministic fake-transport tests for prompt, denial, buffer-zeroing, opaque attributes, and secret-nondisclosure boundaries.
- Kept disposable-user physical Secret Service validation and server KMS integration explicitly open.

## 0.5.7-dev -- macOS Keychain Credential Store

- Added a native macOS Keychain backend through Security.framework `SecItem` APIs with no shell-command or process-argument secret transport.
- Preserved opaque service identifiers, the 2,048-byte value bound, idempotent deletion, missing-item semantics, and the no-plaintext-fallback contract.
- Kept temporary C buffers bounded and explicitly zeroed before release, while returning only numeric Keychain status codes on failure.
- Added a hosted macOS compile-and-test gate that does not mutate the runner's Keychain.
- Made the desktop credential-source label platform-neutral.
- Kept disposable-user physical Keychain validation, Linux Secret Service, and server KMS integration explicitly open.

## 0.5.6-dev -- Desktop Provider Credential Wiring

- Connected Windows Credential Manager values to desktop provider discovery and local Gateway construction.
- Added provider credential status, save, replace, and delete operations without exposing stored values to the frontend.
- Preserved environment-variable precedence and denied changes while the local Gateway is starting or running.
- Disabled implicit local-echo fallback when any real provider credential is available and verified Authorization-header delivery with an in-process provider fixture.
- Added a 2,048-byte boundary at both the desktop service and native store, cleared submitted UI secrets after every attempt, and added accessible error/status relationships.
- Kept physical Windows credential-store smoke, native macOS Keychain, Linux Secret Service, and server KMS integration explicitly open.

## 0.5.5-dev -- Windows Credential Store Foundation

- Added a native Windows Credential Manager adapter behind the existing credential-store port without a plaintext fallback.
- Minimized operating-system metadata with namespace-scoped SHA-256 targets instead of provider, account, or logical credential names.
- Bounded values to 2,048 bytes, copied caller/backend buffers, and zeroed transient adapter values.
- Kept missing deletion idempotent and mapped missing reads to the existing `core.ErrNotFound` contract.
- Added denial and isolation tests for invalid namespaces, references, sizes, cancellation, missing values, backend failures, and secret-free errors.
- Kept macOS Keychain, Linux Secret Service, desktop provider wiring, physical-device validation, and server KMS integration explicitly open.

## 0.5.4-dev -- OIDC Control Identity

- Added provider-neutral OIDC discovery and ID-token verification for Control API bearers.
- Required exact issuer, audience, authorized party, signature, expiry, not-before, subject, and explicit provider-role mapping before creating a `control-access/v1` principal.
- Preserved digested static credentials alongside OIDC for Gateway and bootstrap use.
- Pseudonymized OIDC subjects before authorization audit events and rejected oversized or structurally invalid bearer tokens.
- Validated the configured issuer, redirects, and discovered JWKS endpoint against the SSRF policy and dialed validated addresses to reduce DNS-rebinding exposure.
- Added live TLS discovery, denial, scoped-privilege, private-address, and JWKS rotation regression tests.
- Kept browser authorization-code, nonce, PKCE, session-cookie, membership, and revocation work explicitly outside the API bearer boundary.

## 0.5.3-dev -- Control Plane Access Foundation

- Replaced the one-token/one-authority Control boundary with a central deny-by-default `control-access/v1` policy.
- Added separate cluster `gateway`, scoped `viewer`, cluster `publisher`, and cluster `admin` roles.
- Added strict SHA-256 token-digest configuration so servers do not store raw static bearer tokens.
- Limited tenant/project viewers to models allowed by matching virtual keys and removed internal provider account IDs from their responses.
- Kept full snapshot read, watch, signing-key read, and publication behind explicit cluster roles.
- Added safe structured audit events for authentication and authorization decisions without bearer tokens or token digests.
- Preserved loopback-only development without credentials and the legacy `CONTROL_BEARER_TOKEN` cluster-admin compatibility path.
- Added denial-first role, cross-project visibility, global publication, authentication, audit, and strict-config regression tests.

## 0.5.2-dev -- Verified Local Release Gates

- Closed the hosted CI chain for Linux race/vet, PostgreSQL 18 integration, Buf and SQLC generation, generated contracts, both Svelte workspaces, and Windows Wails desktop/MCP compilation.
- Patched `@sveltejs/kit` to 2.70.2 for GHSA-29g2-3rmr-qm68 and added a lockfile regression gate.
- Upgraded `actions/checkout` to Node 24-based v7.0.1 and added local Go-format, Buf, SQLC, and generated-contract verification intents.
- Adopted the ssealed minimal-monorepo profile with desktop-app and cli-tool addons.
- Finalized the public module as `github.com/0disoft/relaydock` and unified binary, archive, package, Wails, and protocol identities under RelayDock.
- Migrated new local-state, IPC, and autostart identifiers while retaining read compatibility for legacy state directories.
- Added a fail-closed release-readiness gate for version, module, lockfiles, license, and source manifests.
- Added job-scoped SLSA provenance for generated contracts, source, server, web, and Windows desktop artifacts.
- Added pinned Syft SPDX JSON and SBOM attestations for matching checksum subjects.
- Added six unpublished Linux/amd64 OCI release candidates with image digests, archive checksums, SBOMs, and attestations.
- Added domain-separated Ed25519 checksum signing and verification with an independent release key.
- Added strict, no-overwrite updater-manifest signing over one canonical verifier payload.
- Made portable and Windows bundles fail closed on the final Apache-2.0 `LICENSE` and `NOTICE`.
- Finalized the repository and distributed artifacts as Apache-2.0, keeping private managed-service code in separate repositories.
- Generated Go and Bun dependency locks and fixed Bun lifecycle-script trust to an empty allowlist.
- Fixed frontend build false positives and switched the Wails desktop bridge to bundler-compatible `@wailsio/runtime`.
- Applied user/SYSTEM/administrator-only DACLs to Windows local Expert state and atomic files.
- Stabilized outbox storage failures as a nonleaking 503 envelope.
- Pinned Bun 1.3.14 and TypeScript 7, with real `svelte-check` gates.
- Preserved root packaging source while excluding nested frontend build/typecheck output from source archives.
- Standardized read-only Go resolution and frozen root-workspace Bun installs in CI and release workflows.
- Required `go.sum`, read-only module resolution, and non-root runtime behavior in every service image.
- Enforced one version/commit/build-time envelope in Go buildinfo and OCI labels.
- Kept ssealed product, architecture, and operations pages as thin routes to the numbered authority documents.
- Normalized Windows source modes to `0644` for regular files and `0755` for shell scripts, with Windows and POSIX regressions.

## 0.5.0-dev -- Bounded Modules, Chunked Local State, and Key Rotation

### Bounded Repository and Modularization

- Enforced a 40 KiB default ceiling for regular files with explicit path-and-reason exceptions.
- Split the large manifest into a root index and approximately 30 KiB chunks.
- Verified chunk paths, ranges, ordering, hashes, aggregate totals, and repository parity.
- Added deterministic source ZIP creation with sorted entries and normalized timestamps.
- Failed release creation on source mode/size/hash races and repository-internal output paths.
- Rejected silent omission of symlinks, sockets, devices, and other nonregular files.
- Added strict JSON parsing and safe backup-swap publication for manifests and size policy.
- Split candidate sources, HTTP ingress, provider adapters, runtime Gateway logic, and the local Expert store by responsibility.

### Local Expert Store v3

- Moved ContextPack source into immutable SHA-256 chunks of at most 32 KiB.
- Created ContextPacks and consultations in one metadata transaction.
- Migrated v1/v2 embedded stores to v3 while retaining `.v2.bak`.
- Verified every chunk digest at store open and synchronized reads/writes with destructive compaction.
- Added immutable-ID conflict checks, retention, orphan cleanup, dry-run, statistics, and `expertstorectl`.
- Made PostgreSQL ContextPack and consultation creation transactional as well.

### Signing and Token Rotation

- Added `signingKeyId`, active/retiring Ed25519 trust, and persisted-snapshot resigning.
- Added explicit retirement switches for legacy no-key-ID snapshots.
- Introduced `argt2.<key-id>.<claims>.<hmac>` Remote MCP tokens.
- Issued only with the active HMAC key while verifying retiring keys.
- Bounded key counts, secret sizes, identities, scopes, and payloads; rejected normalized key-ID collisions.
- Kept argt1 verification behind an explicit, removable compatibility switch.
- Rejected reuse of an active key ID with a different secret.

### Release and CI

- Connected `releasepack audit`, `verify`, and `build` to Task, checks, CI, and release workflows.
- Added deterministic source archives, external SHA-256 artifacts, server operations binaries, and regression coverage for size, archive, store, and rotation contracts.

## 0.4.0-dev -- Signed Control, Durable Usage, and Scoped MCP

### Signed Control Distribution

- Verified Ed25519 snapshots and applied route tables atomically.
- Used verified LKG during Control outages, then failed readiness and new requests at expiry.
- Rejected same-revision mutation, rollback, and invalid signatures.
- Added local atomic and PostgreSQL immutable snapshot stores with serialized publication and replica-race recovery.
- Added secret-manager signing material and `controlctl` publication/inspection commands.

### Runtime Journal and Outbox

- Added PostgreSQL request, provider-attempt, usage, and error journaling.
- Made request completion and outbox insertion atomic and idempotent.
- Added leased, fenced, bounded-concurrency outbox delivery with Retry-After, backoff, and dead letters.
- Added signed nonredirecting webhooks and a reference consumer for replay, duplicate, and payload-conflict checks.
- Added `outboxctl` status, requeue, and purge operations.

### Remote MCP Security

- Added audience, token ID, subject, tenant, project, scope, issue, and expiry claims.
- Separated read, answer, and administrative scopes and enforced tenant/project parity through consultations and ContextPacks.
- Prevented automatic reuse of the Broker bearer when scoped tokens are configured.
- Kept legacy static tokens behind explicit compatibility settings.

### Policy, HA, CI, and Development

- Made signed snapshots authoritative, removed implicit routes, atomically applied price revisions, and default-blocked direct models.
- Added PostgreSQL watch recovery, integration tests, generated-contract compilation, Windows Wails compilation, and a complete development Compose stack.

## 0.3.0-dev -- Durable Gateway and Expert Runtime

- Added environment-based provider composition, virtual and direct routes, stable provider-error classification, safe retries, cooldown, live SSE forwarding, and Valkey leases.
- Added local and PostgreSQL Expert persistence, worker claim/renewal/recovery/backoff/fencing, API execution, web handoff, and result provenance.
- Added durable Control snapshots, virtual-key issue/authenticate/revoke, organization/project bootstrap, and the `zg_...` key format.
- Added embedded migrations, operations commands, route examples, and tests for routing, streaming, workers, durability, Control, keys, migrations, and leases.

## 0.1.0-dev -- Implemented Vertical Slice

- Added OpenAI Responses/Chat, Anthropic Messages, and Gemini translation with canonical models and loss reports.
- Treated SSE/NDJSON EOF without terminal events as failure and prohibited transparent post-semantic retries.
- Added the local `local/echo` JSON and SSE Gateway.
- Added ContextPack selection, digests, traversal defense, redaction, consultation lifecycle, idempotency, cancellation, result storage, API execution, and manual ChatGPT handoff.
- Added Wails v3 single-instance, tray, close-to-tray, and `--background` autostart.
- Added the Codex MCP STDIO bridge, Named Pipe/Unix Domain Socket runtime, and a Wails UI for previews, consultations, API execution, and web-result import.
