# 24. Development Stack

## 목적

`deploy/docker-compose.dev.yml`은 실제 production manifest가 아니라 PostgreSQL durability, Control snapshot, Gateway journal, outbox webhook의 로컬 수직 슬라이스를 반복 실행하기 위한 개발 스택이다.

## 구성

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

개발 스택에는 재현 가능한 고정 UUID, 고정 Ed25519 seed, 고정 HMAC secret이 들어간다. 이 값은 로컬 contract test 전용이며 production에서 재사용하면 안 된다.

## 시작

```powershell
docker compose -f deploy/docker-compose.dev.yml up --build
```

Outbox 왕복까지 포함하려면 profile을 켠다.

```powershell
docker compose -f deploy/docker-compose.dev.yml --profile outbox up --build
```

## 확인

```powershell
Invoke-WebRequest http://127.0.0.1:8080/healthz
Invoke-WebRequest http://127.0.0.1:8081/readyz
Invoke-WebRequest http://127.0.0.1:8082/readyz
Invoke-WebRequest http://127.0.0.1:8083/healthz
Invoke-RestMethod -Headers @{ Authorization = "Bearer dev-sink-admin" } http://127.0.0.1:8090/events
```

## PostgreSQL integration test

테스트는 지정된 DB의 product table을 truncate하므로 전용 일회성 database만 사용한다.

```powershell
$env:ARG_TEST_POSTGRES_URL = "postgres://airuntime:airuntime@127.0.0.1:5432/airuntime_test?sslmode=disable"
$env:ARG_TEST_POSTGRES_ALLOW_RESET = "I_UNDERSTAND_THIS_DATABASE_WILL_BE_TRUNCATED"
go test -tags=integration -count=1 ./tests/postgres
```

이 테스트는 migration, signed snapshot history, request→attempt→usage→outbox transaction, 동일 쓰기 멱등성, payload conflict, lease expiry fencing, stale claim reclaim을 검증한다.

## 초기화

모든 개발 데이터를 지우려면 compose를 내리고 volume을 삭제한다.

```powershell
docker compose -f deploy/docker-compose.dev.yml down -v
```

개별 migration row나 snapshot row를 수동 수정해 테스트를 살리지 않는다. 재현이 필요하면 새 volume에서 다시 시작한다.
