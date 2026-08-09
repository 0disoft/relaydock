# 20. Operations Runbook

This runbook defines initialization, rollout, failure diagnosis, and recovery for managed servers. Commands use PowerShell; on Linux, adapt only environment-variable syntax.

## 1. Initialize PostgreSQL

```powershell
$env:ARG_POSTGRES_URL = "postgres://airuntime:airuntime@127.0.0.1:5432/airuntime?sslmode=disable"
go run ./cmd/dbmigrate status
go run ./cmd/dbmigrate up
```

The migration runner holds an advisory lock and uses one transaction per migration. It stops when an applied SQL checksum differs from the file. Add a new version; never edit an applied migration.

## 2. Create an Organization, Project, and Virtual Key

```powershell
$project = go run ./cmd/projectctl ensure `
  --organization-slug default `
  --organization-name "Default Organization" `
  --project-slug default `
  --project-name "Default Project" | ConvertFrom-Json

$pepper = New-Object byte[] 32
[Security.Cryptography.RandomNumberGenerator]::Fill($pepper)
$env:GATEWAY_VIRTUAL_KEY_PEPPER_B64 = [Convert]::ToBase64String($pepper)
$env:GATEWAY_KEY_ENVIRONMENT = "live"

$key = go run ./cmd/keyctl issue `
  --tenant $project.organizationId `
  --project $project.projectId `
  --scopes "models:invoke" `
  --models "code-fast,code-deep" `
  --expires 720h | ConvertFrom-Json
```

Losing the pepper makes existing keys unverifiable, and plaintext keys cannot be read again. Store them separately.

## 3. Establish the Control Plane Trust Root

One replica may use a local store. Two or more replicas require PostgreSQL and one shared signing secret.

```powershell
$env:CONTROL_SNAPSHOT_STORE = "postgres"
$env:CONTROL_POSTGRES_URL = $env:ARG_POSTGRES_URL
$env:CONTROL_SIGNING_PRIVATE_KEY = "<base64url-ed25519-seed>"
$env:CONTROL_ACCESS_TOKENS_JSON = '<gateway, viewer, publisher, and admin token-digest records>'
go run ./cmd/controld

$publicKey = go run ./cmd/controlctl signing-key --raw
```

Without `CONTROL_SIGNING_PRIVATE_KEY`, Control creates a per-instance file at `CONTROL_SIGNING_KEY_PATH`. Do not use per-replica files in PostgreSQL HA mode. Follow [`28-control-plane-access.md`](28-control-plane-access.md) to generate separate credentials. `CONTROL_BEARER_TOKEN` remains only as a legacy cluster-admin bootstrap path.

## 4. Publish Routes and Synchronize Gateway

```powershell
go run ./cmd/controlctl publish --file config/runtime-snapshot.example.json
go run ./cmd/controlctl snapshot --output snapshot.json
go run ./cmd/controlctl models

$env:GATEWAY_CONTROL_URL = "http://127.0.0.1:8081"
$env:GATEWAY_CONTROL_BEARER_TOKEN = "<raw-token-for-a-gateway-role-credential>"
$env:GATEWAY_CONTROL_SIGNING_PUBLIC_KEY_B64 = $publicKey
$env:GATEWAY_CONTROL_REQUIRED = "true"
$env:GATEWAY_CONTROL_LKG_PATH = "data/gateway/runtime-snapshot.json"
```

Gateway validates signature, revision, generation, and expiry, and applies only higher revisions. It uses LKG during a Control outage but fails readiness and new requests after snapshot expiry. Correct a bad route with a higher revision; never update the existing row.

## 5. Start Expert Broker and Remote MCP

```powershell
$env:EXPERT_POSTGRES_URL = $env:ARG_POSTGRES_URL
$env:EXPERT_TENANT_ID = $project.organizationId
$env:EXPERT_PROJECT_ID = $project.projectId
$env:EXPERT_BROKER_BEARER_TOKEN = "<broker-admin-token>"
$env:EXPERT_MCP_TOKEN_SECRET = "<at-least-32-random-bytes>"
$env:EXPERT_MCP_ALLOW_BROKER_TOKEN = "false"
go run ./cmd/expert-brokerd

go run ./cmd/mcptokenctl issue `
  --subject chatgpt-reviewer `
  --tenant $project.organizationId `
  --project $project.projectId `
  --scopes "consultations:read,consultations:answer" `
  --ttl 1h
```

Do not share Broker administrator and MCP reviewer tokens. `consultations:admin` is cluster-wide and must not be issued to automated reviewers.

## 6. Enable the Gateway Journal

```powershell
$env:GATEWAY_POSTGRES_URL = $env:ARG_POSTGRES_URL
$env:GATEWAY_RUNTIME_JOURNAL = "postgres"
$env:GATEWAY_PRICE_REVISION_ID = "price-2026-08"
```

When journaling operator-bearer requests, configure canonical tenant, project, and virtual-key UUIDs. Normal virtual-key requests derive them from authentication context. A begin-record failure rejects before calling a provider; attempt or completion-record failures remain explicit durability errors.

## 7. Deliver the Outbox

```powershell
$env:OUTBOX_POSTGRES_URL = $env:ARG_POSTGRES_URL
$env:OUTBOX_WEBHOOK_URL = "https://money.internal.example/events"
$env:OUTBOX_WEBHOOK_SECRET = "<random-secret>"
$env:OUTBOX_WORKER_ID = "outbox-seoul-01"
go run ./cmd/outboxd

go run ./cmd/outboxctl status
go run ./cmd/outboxctl list --state dead
```

Receivers validate event-ID idempotency, the timestamp window, and HMAC signatures. Never use `OUTBOX_ALLOW_INSECURE_HTTP=true` outside loopback development. Before requeueing dead letters, diagnose downstream schema and failure causes; never mutate event rows or reuse an ID with a different payload.

## 8. Recommended Startup Order

```text
PostgreSQL
  -> migration one-shot job
  -> project/key bootstrap
  -> Valkey
  -> controld
  -> route publish
  -> expert-brokerd
  -> gatewayd
  -> outboxd
  -> control-console
```

Gateway is not ready without a valid required snapshot. Outbox may start later and reclaim committed events. Invalid operator audit UUIDs are rejected at process startup.

## 9. Health and Readiness

```powershell
Invoke-WebRequest http://127.0.0.1:8080/healthz
Invoke-WebRequest http://127.0.0.1:8080/readyz
Invoke-WebRequest http://127.0.0.1:8081/readyz
Invoke-WebRequest http://127.0.0.1:8082/readyz
Invoke-WebRequest http://127.0.0.1:8083/healthz
```

- `healthz` means the process is alive and accepting HTTP.
- `readyz` includes required dependencies and snapshot validity.
- Isolate one provider failure as candidate cooldown, not process unreadiness.
- Requests fail with `no route` when every candidate is unavailable.
- An expired Control snapshot lowers Gateway readiness.

## 10. Worker Failures

For Expert jobs stuck in `running`, inspect `locked_by`, `lock_expires_at`, `attempt_count`, `available_at`, provider error class, and Retry-After. Another worker reclaims after expiry; fencing rejects late results. Confirm a worker is dead before clearing locks manually.

For Outbox failures, inspect the lock owner and expiry, `Retry-After`, `available_at`, and `dead_at`. Completion after lease expiry is rejected. Receivers absorb lost responses through event-ID idempotency.

## 11. Valkey Failure

Never treat failed lease commands as success. New provider attempts fail closed. Do not switch only some Gateways to memory leases, mix memory and Valkey within one account pool, or use Valkey as recovery data for usage, consultations, or balances. Emergency changes stop all Gateways and restart them in one mode.

## 12. Migration Rollback

```powershell
go run ./cmd/dbmigrate status
go run ./cmd/dbmigrate -steps 1 down
```

Create a backup and restore point first. Destructive down migrations are not an automatic recovery mechanism, and applied SQL is never edited to bypass a checksum mismatch.

## 13. Revoke a Virtual Key

```powershell
go run ./cmd/keyctl revoke `
  --key-id "<virtualKeyId>" `
  --project "<projectId>"
```

Also assess exposure of provider credentials, operator bearers, and MCP secrets.

## 14. Minimum Backup Scope

Back up PostgreSQL plus WAL/PITR, Control signing material, Gateway LKG, provider credentials, virtual-key pepper, Remote MCP key material, and the outbox receiver HMAC secret. Do not treat Valkey lease/cooldown state, process-local health scores, or expired ContextPacks as recovery targets.

## 15. Incident Triage

| Symptom | Check first |
|---|---|
| All requests return 401 | Bearer/key format, pepper, environment, revocation, expiry |
| One model has no route | Snapshot route, credential, cooldown, capability mismatch |
| Gateway `readyz` is 503 | Control signature, LKG, snapshot expiry, database auth dependency |
| Stream ends early | Provider terminal event, reset, cancellation, semantic commit |
| Consultation keeps retrying | Provider error, maximum attempts, cost ceiling, lease renewal |
| Result commit rejected | Worker fencing, tenant/project mismatch, completed consultation |
| Dead outbox grows | Receiver 4xx, signature mismatch, schema incompatibility, max attempts |
| Migration rejected | Checksum mismatch, advisory lock, database privilege |
| Control signature error | Shared signing secret and Gateway public-key mismatch |

## 16. Pre-Release Failure Drills

- Inject provider 429, timeout, and connection resets.
- Gracefully stop Gateway with active streams.
- Stop Control, use LKG, then observe readiness at expiry.
- Initialize and publish from concurrent Control replicas.
- Kill Expert and Outbox workers, then verify reclaim and lease fencing.
- Restart and partition Valkey.
- Exercise PostgreSQL read-only mode, connection exhaustion, and failover.
- Stop the process during a Wails update.
- Revoke or rotate leaked virtual keys and MCP tokens.
- Restore a backup into a separate environment.

## 17. Key Rotation and Local Maintenance

Deploy old and new public keys to both Control and Gateway trust rings before switching the active Control key. Publish a new revision, confirm every Gateway reports the new key ID, retain the old key through the maximum snapshot/LKG lifetime, then disable legacy no-key-ID support. Follow `docs/26-signing-and-token-key-rotation.md`.

```powershell
go run ./cmd/controlctl signing-key --trust-entry
go run ./cmd/expertstorectl verify --state data/expert-broker/state.json
go run ./cmd/expertstorectl stats --state data/expert-broker/state.json
go run ./cmd/expertstorectl compact --state data/expert-broker/state.json --dry-run
```

Stop desktop/headless writers before manual compaction. Review dry-run output first, and retain `.v2.bak` until create/read/restart validation succeeds.

## 18. Source Archive and Size Audit

```powershell
go run ./cmd/releasepack audit --root .
go run ./cmd/releasepack build --root . --output ../relaydock-source.zip
go run ./cmd/releasepack verify --root .
```

Split handwritten code and documentation over 40 KiB by responsibility. Add only genuinely indivisible upstream fixtures or canonical package-manager locks to `config/file-size-exceptions.json`, with a specific reason.
