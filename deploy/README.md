# Deployment

This directory provides distroless images for server binaries, an operations image for migration and bootstrap tasks, the complete development Compose stack, and an example Caddy reverse proxy. The Wails desktop app is not deployed with Docker.

## Development Compose

```powershell
docker compose -f deploy/docker-compose.dev.yml up --build
```

Default exposure:

| Service | Address | Development authentication |
|---|---|---|
| Gateway | `127.0.0.1:8080` | `development-gateway-token` |
| Control | `127.0.0.1:8081` | `development-control-token` |
| Expert | `127.0.0.1:8082` | `development-expert-token` |
| PostgreSQL | `127.0.0.1:5432` | Development account in the Compose file |
| Valkey | `127.0.0.1:6379` | None |

These values are for local use only. Do not reuse them on an external host or shared development server.

Compose runs `dbmigrate up` once after PostgreSQL becomes healthy, then starts the Gateway. The Expert Broker uses a local atomic store on a writable volume by default. To test the PostgreSQL Expert store, first create tenant and project IDs with `projectctl ensure`, then provide `EXPERT_POSTGRES_URL`, `EXPERT_TENANT_ID`, and `EXPERT_PROJECT_ID`.

Optional Caddy proxy:

```powershell
docker compose -f deploy/docker-compose.dev.yml --profile proxy up --build
```

## Images

- `Dockerfile.gateway`: stateless Gateway data plane
- `Dockerfile.control`: includes writable Control snapshot and signing-key directories
- `Dockerfile.expert`: includes a writable local Expert-state directory
- `Dockerfile.ops`: `dbmigrate`, `projectctl`, and `keyctl`

The Control and Expert runtimes use non-root UIDs. Changing state-volume ownership to an arbitrary root UID can prevent restart.

Every service-image build stage copies both `go.mod` and `go.sum` and rejects module-graph changes with `GOFLAGS=-mod=readonly`. The image build does not proceed when `go.sum` is missing or inconsistent with the current `go.mod`.

Release image builds must provide the `VERSION`, `COMMIT`, and `BUILD_TIME` build arguments. The same values must appear in Go `buildinfo` and the OCI `version`, `revision`, and `created` labels. Images that retain the `dev` or `unknown` defaults are not eligible for production promotion.

## Initial Operational Baseline

- Cloudflare: DNS, WAF, TLS, and static assets
- Hetzner: Gateway, Control, and Expert
- PostgreSQL: authoritative state, backups, PITR, and restore rehearsals
- Valkey: replayable lease, cooldown, and rate state only
- R2/S3: encrypted ContextPacks and short-TTL attachments
- Migration: one runner per deployment
- Image: immutable digest pinning and SBOM
- Secrets: never included in Compose files or image layers

See `../docs/20-operations-runbook.md` for detailed failure and recovery procedures.
