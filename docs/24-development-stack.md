# 24. Development Stack

## Purpose

`deploy/docker-compose.dev.yml` is not a production manifest. It is a repeatable development stack for the local vertical slice covering PostgreSQL durability, Control snapshots, the Gateway journal, and outbox webhooks.

## Components

```text
postgres 18
valkey 9
migration one-shot job
seed-dev one-shot job
controld with PostgreSQL snapshot store
expert-brokerd with PostgreSQL store
optional gatewayd
optional outboxd + webhook-sink
```

The stack contains reproducible fixed UUIDs, an Ed25519 seed, and an HMAC secret. They are for local contract tests only and must never be reused in production.

## Start

```powershell
docker compose -f deploy/docker-compose.dev.yml up --build
```

Enable the profile for a full outbox round trip:

```powershell
docker compose -f deploy/docker-compose.dev.yml --profile outbox up --build
```

## Verify

```powershell
Invoke-WebRequest http://127.0.0.1:8080/healthz
Invoke-WebRequest http://127.0.0.1:8081/readyz
Invoke-WebRequest http://127.0.0.1:8082/readyz
Invoke-WebRequest http://127.0.0.1:8083/healthz
Invoke-RestMethod -Headers @{ Authorization = "Bearer dev-sink-admin" } http://127.0.0.1:8090/events
```

## PostgreSQL Integration Test

The test truncates product tables in the configured database. Use only a dedicated ephemeral database.

```powershell
$env:ARG_TEST_POSTGRES_URL = "postgres://airuntime:airuntime@127.0.0.1:5432/airuntime_test?sslmode=disable"
$env:ARG_TEST_POSTGRES_ALLOW_RESET = "I_UNDERSTAND_THIS_DATABASE_WILL_BE_TRUNCATED"
go test -tags=integration -count=1 ./tests/postgres
```

This verifies migrations, signed-snapshot history, the request-to-attempt-to-usage-to-outbox transaction, identical-write idempotency, payload conflicts, lease-expiry fencing, and stale-claim recovery.

## Reset

Stop Compose and remove its volumes to delete all development data:

```powershell
docker compose -f deploy/docker-compose.dev.yml down -v
```

Do not revive a test by editing individual migration or snapshot rows. Reproduce it from a fresh volume.
