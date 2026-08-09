# 21. Control Snapshot Distribution

## Purpose

This contract lets multiple Gateway instances use the same model routes and price revision without querying the Control Plane or PostgreSQL for every request. The data plane consumes only signed immutable snapshots and may continue for a bounded period with the last verified snapshot during a Control Plane failure.

## Authority Boundary

```text
Control operator
  -> controld publish
  -> immutable revision store
  -> Ed25519 signed snapshot
  -> gatewayd verifier
  -> atomic in-memory route swap
  -> last-known-good file
```

Only the Control Plane publishes revisions. A Gateway neither modifies a snapshot nor invents missing candidates. Applying a snapshot removes every undocumented route, including implicit development `local/echo`. With PostgreSQL, `control.runtime_snapshots` is append-only history and the highest committed revision is current.

## Signing Keys

`CONTROL_SIGNING_PRIVATE_KEY` accepts a base64 or base64url-encoded 32-byte Ed25519 seed or 64-byte private key. Without it, Control uses a per-instance file at `CONTROL_SIGNING_KEY_PATH`.

All Control replicas must receive the same secret-manager value. Per-replica local keys produce inconsistent signatures even with a shared PostgreSQL snapshot store.

Read the public key with:

```powershell
go run ./cmd/controlctl signing-key --raw
```

Provide only `GATEWAY_CONTROL_SIGNING_PUBLIC_KEY_B64`, never the private key, to Gateway.

## Publication Rules

- Revisions are positive and strictly increasing.
- Reject republishing the same revision and rollback.
- Snapshots contain generation and expiration times, model routes, provider references, and price revisions.
- When price revisions exist, at least one must already be effective; atomically apply the newest effective revision with routes.
- Sign the canonical unsigned payload.
- Serialize PostgreSQL publication with a serializable transaction and advisory lock.
- During concurrent initialization or updates, losing replicas reread the latest committed revision and continue normally.

## Gateway Application Rules

Gateway starts in this order:

1. Validate the Control endpoint, public key, and configuration.
2. Fetch the current remote snapshot and validate signature, time, and revision.
3. If the remote fetch fails, validate the last-known-good file.
4. Atomically apply a valid snapshot to the route source.
5. Apply only higher revisions from the watch stream.
6. Atomically replace the last-known-good file only after application succeeds.

Reject same-revision content changes, lower revisions, and bad signatures while retaining current routes. Replace the active price revision in the same critical section so route and journal price revisions cannot diverge.

## Expiration and Readiness

With `GATEWAY_CONTROL_REQUIRED=true`, reject `/readyz` and new model requests without a valid snapshot. Never use an applied snapshot after `expiresAt`; a prolonged Control outage fails closed when LKG expires.

An excessively long TTL preserves compromised or retired policy. Use a short, renewable TTL that still covers deployment and recovery time.

## Relationship to Local Routes

`GATEWAY_ROUTES_FILE` and `GATEWAY_ROUTES_JSON` are for bootstrap and single-node development. A signed snapshot becomes authoritative for the full route table. With `GATEWAY_CONTROL_URL`, direct `provider/model` calls are blocked unless `GATEWAY_ALLOW_DIRECT_MODELS=true`. Keep this disabled in production and expose signed virtual models only.

## Recovery

- Control database failure: existing replicas and Gateways use the latest valid revision.
- Lost signing key: restore from backup or perform explicit key rotation; the old trust root cannot sign new revisions otherwise.
- Corrupt LKG: redownload from healthy Control, or lower readiness when Control is also unavailable.
- Bad publication: issue a higher corrective revision; never update or delete a published row.
