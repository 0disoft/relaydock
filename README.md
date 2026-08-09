# RelayDock

RelayDock is a Go-first AI Runtime Gateway that connects coding agents such as Codex, Claude Code, and OpenCode with official AI APIs, self-hosted models, and an Expert Escalation MCP path for difficult architecture reviews.

`0.5.2-dev` is a reference implementation that connects a Wails v3 local runtime, compatible API gateway, multi-provider router, signed Control snapshots, a durable request journal, transactional outbox, and scoped Remote MCP. The `local/echo` vertical slice runs without external infrastructure; PostgreSQL and Valkey enable managed paths.

This is not full production certification. Real provider accounts, a complete Go 1.26 module build, live PostgreSQL and Valkey, native Wails packaging and code signing, and money-platform settlement still require target-environment validation. [`VALIDATION.md`](VALIDATION.md) is the source of truth for executed checks.

## Architecture

```text
Codex / Claude Code / OpenCode
          | MCP stdio
          v
cmd/mcp-bridge
          | Named Pipe / Unix Domain Socket
          v
Wails v3 Desktop or cmd/headless
          +-- local provider gateway
          +-- ContextPack compiler and redaction
          +-- consultation client
          +-- per-user credential boundary

OpenAI Responses / Chat / Anthropic Messages / Gemini clients
          | HTTP + SSE
          v
cmd/gatewayd
          +-- protocol compiler and virtual-model router
          +-- provider adapters and pre-semantic retry
          +-- memory or Valkey leases
          +-- PostgreSQL request journal
          +-- transactional outbox
                     | signed webhook
                     v
                 cmd/outboxd -> money-platform / audit consumer

cmd/controld -- signed snapshot + LKG --> cmd/gatewayd

cmd/expert-brokerd
          +-- local atomic-file or PostgreSQL store
          +-- worker lease, renewal, fencing, and retry
          +-- OpenAI Responses expert executor
          +-- ChatGPT web handoff
          +-- tenant-scoped Remote MCP
```

## Implemented Paths

### Gateway and Protocols

- OpenAI Responses, OpenAI Chat Completions, Anthropic Messages, and Gemini GenerateContent ingress
- Canonical request, response, and stream models with strict loss validation
- OpenAI, Anthropic, Google, DeepSeek, OpenRouter, and OpenAI-compatible adapters
- Virtual-model routes and optional direct `provider/model` calls
- Capability, region, cost, availability, health, queue, and affinity routing
- Provider-error taxonomy and Retry-After handling
- Retry before the first semantic event only
- Live SSE forwarding, terminal validation, and midstream EOF failure
- Process-local cooldown and memory or Valkey Lua concurrency leases

### Signed Control Distribution

- Ed25519 snapshot fetch, watch, and verification
- Key IDs with overlapping old and new trust for rotation
- Atomic application of strictly increasing revisions
- Rejection of same-revision mutation and rollback
- Verified last-known-good fallback and fail-closed expiry
- Atomic route and effective price-revision swaps
- Local atomic-file or PostgreSQL immutable snapshot history
- Multi-replica initialization and update-race recovery

### Runtime Journal and Outbox

- Separate user requests and provider attempts
- Per-attempt provider, model, protocol, semantic commit, usage, and errors
- Total usage across retries
- Atomic request completion and outbox insertion
- Write idempotency and payload conflicts
- Leases, stale-worker fencing, bounded retry, and dead letters
- HMAC webhooks, replay-window validation, explicit duplicate acknowledgements, and idempotency keys

### Expert Escalation and MCP

- File selection, symlink and traversal defense, digests, and Git revisions
- Redaction for environment files, API keys, JWTs, private keys, database URLs, and high-entropy strings
- Consultation idempotency and state machines
- Local atomic-file or PostgreSQL tenant/project persistence
- `FOR UPDATE SKIP LOCKED` claims, renewal, recovery, backoff, and fenced result commits
- API-model execution, web handoff, and preserved Remote MCP provenance
- Codex MCP STDIO bridge
- Separate read, answer, and administrative Remote MCP scopes
- Tenant/project and ContextPack-scope enforcement
- `argt2` key-ID tokens with retiring HMAC keys and explicit legacy retirement

### Desktop and Operations

- Wails v3 tray, single instance, close-to-tray, and background autostart
- Separate Expert metadata and immutable 32 KiB ContextPack chunks
- Shared provider composition in desktop and headless runtimes
- Windows Named Pipe and Unix Domain Socket IPC
- PostgreSQL migrations with versions, checksums, advisory locks, and transactions
- Organization/project bootstrap and virtual-key issue/revoke CLIs
- Unambiguous `zg_<environment>_<public-id>.<secret>` virtual keys
- Authentication enforcement on external binds
- PostgreSQL 18 integration and contract-generation release gates
- A 40 KiB file policy, strict chunked manifest, and deterministic source ZIP

## Executables

| Executable | Responsibility |
|---|---|
| Root Wails app | Tray, settings, ContextPack approval, consultations, local Gateway and IPC |
| `cmd/mcp-bridge` | Minimal MCP STDIO to local runtime bridge |
| `cmd/headless` | Local and CI runtime without a GUI |
| `cmd/gatewayd` | Compatible ingress, routing, streaming data plane, and journal |
| `cmd/controld` | Publish, read, and watch signed runtime snapshots |
| `cmd/controlctl` | Read keys/snapshots/models and publish policy |
| `cmd/expert-brokerd` | ContextPack, consultation, worker, and Remote MCP APIs |
| `cmd/mcptokenctl` | Issue tenant/project-scoped Remote MCP tokens |
| `cmd/expertstorectl` | Verify, inspect, and compact the local Expert store |
| `cmd/releasepack` | Audit size, verify manifests, and create deterministic source ZIPs |
| `cmd/outboxd` | Claim and deliver PostgreSQL outbox events |
| `cmd/outboxctl` | Inspect, requeue, and purge outbox events |
| `cmd/webhook-sink` | Reference consumer for signature and idempotency checks |
| `cmd/dbmigrate` | Apply embedded PostgreSQL migrations |
| `cmd/projectctl` | Bootstrap organizations and projects |
| `cmd/keyctl` | Issue and revoke scoped virtual keys |

## Toolchain Baseline

- Go 1.26
- Wails v3 `v3.0.0-alpha2.119`
- MCP Go SDK `v1.6.1`
- Connect Go `v1.20.0`
- pgx `v5.10.0`
- valkey-go `v1.0.76`
- Svelte `5.56.4`
- Vite `8.1.5`
- TypeScript `7.0.2`
- UnoCSS `66.7.5`

Wails v3 is alpha software and remains exactly pinned. Upgrade only after protocol conformance, worker/outbox fencing, Wails lifecycle, and MCP bridge regression checks.

## Quick Start Without External Infrastructure

The public Go module is `github.com/0disoft/relaydock`. Use `scripts/rename-module.ps1` only when distributing a fork under another module path.

```powershell
./scripts/bootstrap.ps1
go mod tidy
bun install
./scripts/generate.ps1
go run ./cmd/gatewayd
```

Without provider keys or routes, only `local/echo` is enabled:

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri http://127.0.0.1:8080/v1/responses `
  -ContentType application/json `
  -Body '{"model":"local/echo","input":"Review the ledger","stream":false}'
```

Start the local Expert Broker with:

```powershell
go run ./cmd/expert-brokerd
```

Without PostgreSQL it uses `data/expert-broker/state.json`. Setting both `OPENAI_API_KEY` and `EXPERT_MODEL` enables the approved API consultation worker; otherwise web handoff and manual result import remain available.

## Development Stack

```powershell
docker compose -f deploy/docker-compose.dev.yml up --build
docker compose -f deploy/docker-compose.dev.yml --profile outbox up --build
```

Fixed Compose UUIDs and signing secrets are for local contract tests only. See [`docs/24-development-stack.md`](docs/24-development-stack.md).

## Verification

Formatting and validation are separate; checks do not rewrite source.

```powershell
./scripts/format.ps1
./scripts/check.ps1
```

PostgreSQL integration tests truncate the configured database and therefore require a dedicated disposable instance:

```powershell
$env:ARG_TEST_POSTGRES_URL = "postgres://airuntime:airuntime@127.0.0.1:5432/airuntime_test?sslmode=disable"
$env:ARG_TEST_POSTGRES_ALLOW_RESET = "I_UNDERSTAND_THIS_DATABASE_WILL_BE_TRUNCATED"
go test -tags=integration -count=1 ./tests/postgres
```

## Production Work Still Open

- Money-platform quote, hold, capture, and usage reconciliation
- Distributed provider health based on real TTFT and TPS probes
- OS Credential Manager, Keychain, and Secret Service adapters
- Remote MCP OIDC/OAuth and workload identity
- Authenticated Control Console mutations and audit/usage views
- Physical-device Wails lifecycle, installers, signing, and rollback
- Real-provider fixtures, billed-usage reconciliation, and 24-hour soak tests
- PostgreSQL partitioning and retention based on measured load

## Non-Negotiable Boundaries

- MCP STDIO writes no logs to stdout.
- Never transparently retry another provider after the first semantic event.
- Never synthesize a completed event from upstream EOF.
- Valkey is not authoritative for balances, consultations, or usage.
- Do not automate ChatGPT web DOMs or extract session cookies.
- Do not silently discard provider reasoning or tool metadata.
- Keep protocol, routing, and money logic out of Wails adapters.
- Never rewrite snapshot or migration history.
- Never edit generated bindings, Protobuf, or sqlc output manually.

## Documentation

- [`IMPLEMENTATION_STATUS.md`](IMPLEMENTATION_STATUS.md)
- [`docs/README.md`](docs/README.md)
- [`docs/02-system-context.md`](docs/02-system-context.md)
- [`docs/15-build-and-release.md`](docs/15-build-and-release.md)
- [`docs/20-operations-runbook.md`](docs/20-operations-runbook.md)
- [`VALIDATION.md`](VALIDATION.md)

## License

RelayDock source and distributed artifacts are available under the Apache License 2.0. Private managed-service implementation remains in separate repositories.
