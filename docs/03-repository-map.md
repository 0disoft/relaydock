# 03. Repository Map

## Root

Contains the Wails v3 desktop entrypoint, workspace metadata, version, and build contract. Do not place protocol, routing, or billing logic here.

## `cmd/`

Contains independent executable entrypoints only.

| Directory | Responsibility |
|---|---|
| `gatewayd/` | API data-plane composition and process lifecycle |
| `controld/` | Signed-snapshot control plane |
| `expert-brokerd/` | Consultation API, worker, and Remote MCP |
| `mcp-bridge/` | STDIO MCP to local IPC bridge |
| `headless/` | Local runtime without a GUI |
| `dbmigrate/` | Embedded migration-runner CLI |
| `projectctl/` | Organization and project bootstrap CLI |
| `keyctl/` | PostgreSQL virtual-key management CLI |

Do not duplicate domain rules in command packages. Keep validation and persistence contracts in `internal/` packages.

## `internal/protocol/`

Wire protocols and the canonical representation. It knows nothing about provider credentials, databases, or the UI.

## `internal/provider/`

Provider-call adapters and the provider-error taxonomy. It does not own routing policy or customer billing.

## `internal/composition/`

Dependency composition for each executable. It assembles environment variables and adapters behind domain interfaces.

## `internal/routing/`

Selects candidates using capability filters, health, cost, affinity, and leases. It does not parse HTTP payloads directly. `distributedlease/` owns the Valkey Lua contract.

## `internal/runtime/`

Owns attempt lifecycle, retry boundaries, lease renewal, and semantic commit. It does not know ingress response formats.

## `internal/expert/`

Owns consultations, ContextPacks, redaction, result contracts, local and PostgreSQL repositories, workers, and expert routes.

## `internal/desktopwails/`

The adapter between Wails and the domain. Wails imports must end here.

## `internal/persistence/`

PostgreSQL, Valkey, atomic-file, object-store, and migration adapters. Domain packages must not expose pgx or Valkey types.

## `internal/transport/`

HTTP ingress, SSE encoding, and authentication middleware. It does not own domain state directly.

## `config/`

Reviewable example routes and deployment settings. Never store real secrets or provider credentials here.

## `proto/`

Public contracts between services. Do not expose database models directly as Proto messages.

## `db/`

Embedded migrations, schema reference, and sqlc queries. Never modify an applied migration.

## `frontend/`

Wails desktop UI. Calls generated bindings through an adapter.

## `web/control-console/`

Managed Control Plane UI. It does not share a deployment boundary with the Desktop UI.

## `tests/`

Protocol conformance, golden streams, fault injection, expert workers, storage, accounting, and load tests. Tests that require real provider credentials belong in a separate opt-in suite.
