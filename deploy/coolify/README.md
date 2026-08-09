# Coolify Deployment

Register Gateway, Control, and Expert as separate services for the initial managed deployment. Keep PostgreSQL independent from application redeployments and operate it with external backups, PITR, and restore rehearsals. Valkey holds only replayable lease and rate-limit state.

## Services

| Service | Image | Persistent data | Public exposure |
|---|---|---|---|
| Migration job | `Dockerfile.ops` | None | Private one-shot |
| Control | `Dockerfile.control` | Signing key and snapshot state | Management network or authenticated endpoint |
| Expert | `Dockerfile.expert` | PostgreSQL recommended; state volume in local mode | Authenticated endpoint |
| Gateway | `Dockerfile.gateway` | None | Public API |
| PostgreSQL | Managed or dedicated | Database volume and backups | Private network only |
| Valkey | Managed or dedicated | Optional AOF; no authoritative data | Private network only |

## First Deployment

1. Establish the image registry and immutable-digest policy.
2. Create private PostgreSQL and Valkey endpoints.
3. Register provider keys, bearer tokens, and the virtual-key HMAC key with the secret provider.
4. Attach persistent volumes for the Control signing key and snapshot path.
5. Run a one-shot `dbmigrate up` job with the `Dockerfile.ops` image.
6. Create the initial organization and project with `projectctl ensure` from the same operations image.
7. Issue the first virtual key with `keyctl issue` and store it in a secure secret manager.
8. Deploy Control and Expert, then verify `/healthz` and `/readyz`.
9. Canary the Gateway and verify a real route and provider, not `local/echo`.
10. Reconcile provider usage with internal usage events after public rollout.

## Required Deployment Checks

- `/healthz` and `/readyz` probes
- A single migration runner
- State-directory write permission for non-root containers
- PostgreSQL backup and restore test
- Bounded Gateway capacity during a Valkey outage
- R2/S3 bucket lifecycle and encryption
- OTLP endpoint and default payload-logging behavior
- Authentication enforcement on external binds
- Caddy or upstream-proxy request-body and idle timeouts that match streaming requirements

## Upgrade

```text
backup checkpoint
  -> ops/dbmigrate up
  -> control
  -> expert
  -> gateway canary
  -> gateway rollout
```

Prevent Coolify automatic redeployment from running the migration job concurrently on multiple instances. The migration runner also uses a PostgreSQL advisory lock, but deployment orchestration must still preserve a single runner.
