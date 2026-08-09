# 12. Data Model

## Schema Ownership

| Schema | Owning module |
|---|---|
| `control` | `controld` and bootstrap CLI |
| `runtime` | `gatewayd` journal |
| `expert` | `expert-brokerd` |
| `outbox` | Transaction producers and `outboxd` |

Migration files are the source of truth for schema history. `db/schema.sql` is the verification snapshot after all migrations are applied.

## Control

- `organizations`
- `projects`
- `virtual_keys`
- `provider_connections`
- `model_routes`
- `runtime_snapshots`

`runtime_snapshots` is revision-append-only. Treat mismatches among payload, signature, generated columns, and expiry columns as corruption.

## Runtime

### Requests

One request from the user's perspective:

- Tenant, project, and virtual-key identity
- Client request ID
- Ingress protocol and virtual model
- Price revision and authorization reference
- State and first semantic-commit time
- Attempt count and total tokens across all attempts
- Terminal error and completion time

### Provider Attempts

Each retry or fallback uses a separate row:

- Request ID and attempt number
- Provider, account, upstream model, and protocol
- Commit status
- Attempt usage and terminal error
- Start and completion time

### Usage Events

Detailed usage rows preserve provider reports and local estimates. Together with request and attempt summaries, they provide reconciliation evidence.

## Expert

- `context_packs`
- `consultations`
- `consultation_results`

A consultation contains tenant/project scope, an idempotency fingerprint, queue availability, worker lease, attempts, failure reason, and result reference. Result commits must pass worker fencing.

## Outbox

`events` stores aggregate-event delivery state:

- Topic, aggregate ID, and JSON payload
- Available time
- Locked worker and lease expiry
- Attempt count and last error
- Published or dead-letter time

The request terminal update and event insertion occur in one transaction. Consumers use the event ID as the idempotency key.

## Boundary

Do not update tables in another schema directly. Use APIs, immutable snapshots, or the transactional outbox for cross-domain transfer. Valkey state never replaces authoritative data in these schemas.
