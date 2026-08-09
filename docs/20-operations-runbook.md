# 20. Operations Runbook

이 문서는 관리형 서버의 초기화, rollout, 장애 판정과 복구 절차를 정의한다. 명령은 PowerShell 기준이며 Linux에서는 환경 변수 문법만 바꾼다.

## 1. PostgreSQL 초기화

```powershell
$env:ARG_POSTGRES_URL = "postgres://airuntime:airuntime@127.0.0.1:5432/airuntime?sslmode=disable"
go run ./cmd/dbmigrate status
go run ./cmd/dbmigrate up
```

Migration runner는 advisory lock을 잡고 migration 하나마다 transaction을 사용한다. 이미 적용된 SQL의 checksum이 파일과 다르면 시작을 중단한다. 적용된 migration을 수정하지 말고 새 version을 추가한다.

## 2. 조직·프로젝트·virtual key

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

Pepper를 잃으면 기존 key를 검증할 수 없고 평문 key는 재조회할 수 없다. 둘을 같은 저장소에 보관하지 않는다.

## 3. Control Plane trust root

단일 replica는 local store를 쓸 수 있다. 두 replica 이상은 PostgreSQL store와 공용 signing secret을 사용한다.

```powershell
$env:CONTROL_SNAPSHOT_STORE = "postgres"
$env:CONTROL_POSTGRES_URL = $env:ARG_POSTGRES_URL
$env:CONTROL_SIGNING_PRIVATE_KEY = "<base64url-ed25519-seed>"
$env:CONTROL_BEARER_TOKEN = "<operator-token>"
go run ./cmd/controld
```

공개키를 Gateway secret에 저장한다.

```powershell
$publicKey = go run ./cmd/controlctl signing-key --raw
```

`CONTROL_SIGNING_PRIVATE_KEY`가 없으면 `CONTROL_SIGNING_KEY_PATH` 파일을 만든다. replica마다 로컬 파일을 쓰면 서명이 달라지므로 PostgreSQL HA 모드에서는 금지한다.

## 4. Route publish와 Gateway sync

```powershell
go run ./cmd/controlctl publish --file config/runtime-snapshot.example.json
go run ./cmd/controlctl snapshot --output snapshot.json
go run ./cmd/controlctl models
```

Gateway 설정:

```powershell
$env:GATEWAY_CONTROL_URL = "http://127.0.0.1:8081"
$env:GATEWAY_CONTROL_BEARER_TOKEN = "<operator-token>"
$env:GATEWAY_CONTROL_SIGNING_PUBLIC_KEY_B64 = $publicKey
$env:GATEWAY_CONTROL_REQUIRED = "true"
$env:GATEWAY_CONTROL_LKG_PATH = "data/gateway/runtime-snapshot.json"
```

Gateway는 서명, revision, 생성·만료 시각을 확인하고 더 높은 revision만 적용한다. Control 연결이 끊기면 LKG를 사용하지만 snapshot이 만료되면 readiness와 신규 요청을 fail-closed 처리한다.

잘못된 route를 발행했으면 row를 UPDATE하지 말고 더 높은 수정 revision을 발행한다.

## 5. Expert Broker와 Remote MCP

관리형 배포에서 local state fallback을 허용하지 않으려면 PostgreSQL URL을 명시한다.

```powershell
$env:EXPERT_POSTGRES_URL = $env:ARG_POSTGRES_URL
$env:EXPERT_TENANT_ID = $project.organizationId
$env:EXPERT_PROJECT_ID = $project.projectId
$env:EXPERT_BROKER_BEARER_TOKEN = "<broker-admin-token>"
$env:EXPERT_MCP_TOKEN_SECRET = "<at-least-32-random-bytes>"
$env:EXPERT_MCP_ALLOW_BROKER_TOKEN = "false"
go run ./cmd/expert-brokerd
```

Reviewer token 발급:

```powershell
go run ./cmd/mcptokenctl issue `
  --subject chatgpt-reviewer `
  --tenant $project.organizationId `
  --project $project.projectId `
  --scopes "consultations:read,consultations:answer" `
  --ttl 1h
```

Broker admin token과 MCP reviewer token을 공유하지 않는다. `consultations:admin`은 cluster-wide 권한이므로 자동화된 reviewer에 발급하지 않는다.

## 6. Gateway journal

```powershell
$env:GATEWAY_POSTGRES_URL = $env:ARG_POSTGRES_URL
$env:GATEWAY_RUNTIME_JOURNAL = "postgres"
$env:GATEWAY_PRICE_REVISION_ID = "price-2026-08"
```

Operator bearer를 사용하면서 journal을 켜면 operator request에 귀속할 tenant, project, virtual-key ID를 지정한다. 일반 virtual key 요청은 인증 context에서 자동으로 가져온다.

Journal 기록 실패는 durability 경계다. 요청 시작 기록 실패는 provider 호출 전에 요청을 거절하고, attempt·완료 기록 실패는 사용자 응답과 함께 명시적 오류로 남긴다.

## 7. Outbox delivery

```powershell
$env:OUTBOX_POSTGRES_URL = $env:ARG_POSTGRES_URL
$env:OUTBOX_WEBHOOK_URL = "https://money.internal.example/events"
$env:OUTBOX_WEBHOOK_SECRET = "<random-secret>"
$env:OUTBOX_WORKER_ID = "outbox-seoul-01"
go run ./cmd/outboxd
```

Receiver는 event ID 멱등성, timestamp window, HMAC signature를 검증해야 한다. `OUTBOX_ALLOW_INSECURE_HTTP=true`는 loopback 개발 이외에 사용하지 않는다.

상태 확인:

```powershell
go run ./cmd/outboxctl status
go run ./cmd/outboxctl list --state dead
```

Dead-letter 재처리 전 downstream schema와 장애 원인을 확인한다. 같은 ID의 다른 payload를 만들거나 event row를 수동 수정하지 않는다.

## 8. 권장 시작 순서

```text
PostgreSQL
  → migration one-shot job
  → project/key bootstrap
  → Valkey
  → controld
  → route publish
  → expert-brokerd
  → gatewayd
  → outboxd
  → control-console
```

`gatewayd`는 `GATEWAY_CONTROL_REQUIRED=true`인데 유효 snapshot이 없으면 준비 상태가 되지 않는다. `outboxd`는 Gateway보다 늦게 시작해도 DB에 committed event를 다시 claim한다.
PostgreSQL journal에서 operator bearer 또는 인증 없는 loopback 경로를 허용할 때는 operator tenant를 명시하고 project·virtual-key ID를 canonical UUID로 설정해야 한다. 잘못된 ID는 첫 요청이 아니라 process 시작 시 거절된다.

## 9. Health와 readiness

```powershell
Invoke-WebRequest http://127.0.0.1:8080/healthz
Invoke-WebRequest http://127.0.0.1:8080/readyz
Invoke-WebRequest http://127.0.0.1:8081/readyz
Invoke-WebRequest http://127.0.0.1:8082/readyz
Invoke-WebRequest http://127.0.0.1:8083/healthz
```

- `healthz`는 process 생존과 HTTP 수신 가능 여부다.
- `readyz`는 필수 dependency와 snapshot 유효성을 포함한다.
- provider 한 곳의 장애는 process readiness 대신 candidate cooldown으로 격리한다.
- 모든 route candidate가 unavailable이면 해당 요청이 `no route`로 실패한다.
- Control snapshot 만료는 Gateway 전체 policy freshness 실패이므로 readiness를 내린다.

## 10. Expert worker 장애

상담이 `running`에 오래 머무르면 `locked_by`, `lock_expires_at`, `attempt_count`, `available_at`, provider error class와 Retry-After를 확인한다.

Worker가 죽으면 lease expiry 뒤 다른 worker가 reclaim한다. 이전 worker가 늦게 돌아와 결과를 쓰면 fencing이 거절한다. 수동으로 lock column을 지우기 전에 해당 worker process가 실제 종료됐는지 확인한다.

## 11. Outbox worker 장애

- `locked_by`와 `lock_expires_at`을 확인한다.
- receiver가 429·503을 반환하면 `Retry-After`와 `available_at`을 확인한다.
- lease 만료 이후 이전 worker의 completion은 성공으로 처리되지 않는다.
- `dead_at`이 설정된 event는 자동 claim 대상에서 제외된다.
- receiver가 처리 완료했는데 응답만 유실된 경우 event ID 멱등성으로 재전송을 흡수해야 한다.

## 12. Valkey 장애

Valkey lease 명령 실패를 성공으로 간주하지 않는다. 장애 중 새 provider attempt는 fail-closed다.

금지:

- 일부 Gateway만 memory lease로 즉시 전환
- 같은 account pool에서 memory와 Valkey lease 혼용
- Valkey 데이터를 usage·상담·잔액 복구 대상으로 취급

긴급 전환은 모든 Gateway를 동시에 정지하고 동일 모드로 재기동한다.

## 13. Migration rollback

```powershell
go run ./cmd/dbmigrate status
go run ./cmd/dbmigrate -steps 1 down
```

Data-destructive down migration은 자동 복구 수단이 아니다. 운영에서는 backup과 restore point를 먼저 만든다. checksum mismatch를 우회하려고 적용된 SQL 파일을 수정하지 않는다.

## 14. Virtual key 폐기

```powershell
go run ./cmd/keyctl revoke `
  --key-id "<virtualKeyId>" `
  --project "<projectId>"
```

유출 사고에서는 virtual key 폐기와 별개로 provider credential, operator bearer, MCP token secret의 노출 범위를 판정한다.

## 15. Backup 최소 범위

필수:

- PostgreSQL base backup과 WAL/PITR
- Control signing seed/private key
- Gateway LKG snapshot
- provider credential secret manager
- virtual-key pepper
- Remote MCP HMAC secret 또는 OIDC key material
- outbox receiver HMAC secret

복구 대상 아님:

- Valkey lease와 cooldown state
- process-local health score
- 만료된 ContextPack object

## 16. 장애 분류

| 증상 | 우선 확인 |
|---|---|
| 모든 요청 401 | bearer·virtual key 형식, pepper, environment, revoke/expiry |
| 특정 모델만 no route | snapshot route, credential, cooldown, capability mismatch |
| Gateway readyz 503 | Control signature·LKG·snapshot expiry·DB auth dependency |
| stream 중간 종료 | provider terminal event, reset, client cancel, semantic commit 여부 |
| 상담 계속 retry | provider error, maximum attempts, cost ceiling, lease renew |
| 결과 commit 거절 | worker fencing, tenant/project mismatch, 이미 완료된 상담 |
| outbox dead 증가 | receiver 4xx, signature mismatch, schema incompatibility, max attempts |
| migration 거절 | checksum mismatch, advisory lock, DB privilege |
| Control signature 오류 | shared signing secret과 Gateway public key mismatch |

## 17. 출시 전 장애 훈련

- provider 429·timeout·connection reset 주입
- Gateway graceful shutdown 중 active stream
- Control 중단 후 LKG 사용, expiry 시 readiness 전환
- Control replica 동시 initialize/publish
- Expert worker kill -9 후 reclaim
- Outbox worker kill -9, exact lease-expiry completion, receiver timeout
- Valkey restart와 network partition
- PostgreSQL read-only·connection exhaustion·failover
- Wails update 중 process 종료
- virtual key와 MCP token 유출 후 revoke/rotation
- backup에서 별도 환경 restore

## Control signing-key rotation

현재 active key의 trust entry를 먼저 확인한다.

```powershell
go run ./cmd/controlctl signing-key --trust-entry
```

신키를 Control에 active로 배포하기 전에 구키와 신키 public key를 Control·Gateway trust ring에 모두 배포한다. 새 revision이 신키로 발행되고 모든 Gateway의 runtime status가 새 signing key ID와 revision을 반영한 뒤에도, 기존 snapshot과 LKG 최대 수명이 끝날 때까지 구키를 유지한다. 마지막으로 legacy no-key-ID 허용을 양쪽에서 끈다. 세부 순서는 `docs/26-signing-and-token-key-rotation.md`를 따른다.

## Local Expert store 점검과 compaction

```powershell
go run ./cmd/expertstorectl verify --state data/expert-broker/state.json
go run ./cmd/expertstorectl stats --state data/expert-broker/state.json
go run ./cmd/expertstorectl compact `
  --state data/expert-broker/state.json `
  --dry-run
```

실행 중인 desktop/headless가 같은 store를 수정하는 동안 수동 compaction을 실행하지 않는다. 프로세스를 정상 종료한 뒤 dry-run 결과를 확인하고 실제 compaction을 실행한다. `.v2.bak`은 migration 직후 자동 삭제하지 말고 새 store로 상담 생성·조회·재시작을 확인한 뒤 운영자가 보존 정책에 따라 제거한다.

## Source archive와 파일 크기 감사

```powershell
go run ./cmd/releasepack audit --root .
go run ./cmd/releasepack build --root . --output ../ai-runtime-gateway-source.zip
go run ./cmd/releasepack verify --root .
```

40 KiB를 넘는 파일을 발견했다고 무조건 예외에 넣지 않는다. 수작업 코드와 문서는 책임별로 분리한다. 외부 conformance binary처럼 byte identity가 계약인 파일만 `config/file-size-exceptions.json`에 이유와 함께 등록한다.
