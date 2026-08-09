# AI Runtime Gateway

Codex·Claude Code·OpenCode 같은 코딩 에이전트, 공식 AI API, 자체 모델 서버, 고난도 설계 검토용 Expert Escalation MCP를 하나의 운영 계층으로 묶는 Go-first 저장소다.

`0.5.0-dev`는 scaffold가 아니라 **Wails v3 로컬 런타임, 호환 API Gateway, 다중 공급자 라우터, 서명된 Control snapshot, 영속 request journal, transactional outbox, scoped Remote MCP까지 연결한 reference implementation**이다. 외부 인프라 없이 `local/echo` 수직 슬라이스를 실행할 수 있고, PostgreSQL·Valkey를 붙이면 관리형 경로를 사용할 수 있다.

완전한 production certification을 주장하지 않는다. 실제 공급자 계정, Go 1.26 전체 module build, PostgreSQL·Valkey 실서버, Wails 네이티브 패키징·코드 서명, money-platform 정산은 대상 환경에서 별도로 통과해야 한다. 실제 검증 범위는 `VALIDATION.md`가 SSOT다.

## 시스템 구성

```text
Codex / Claude Code / OpenCode
          │ MCP stdio
          ▼
cmd/mcp-bridge
          │ Named Pipe / Unix Domain Socket
          ▼
Wails v3 Desktop 또는 cmd/headless
          ├── local provider gateway
          ├── ContextPack compiler
          ├── secret redaction
          ├── consultation client
          └── per-user credential boundary

OpenAI Responses / Chat / Anthropic Messages / Gemini client
          │ HTTP + SSE
          ▼
cmd/gatewayd
          ├── protocol compiler
          ├── virtual-model router
          ├── provider adapters
          ├── pre-semantic retry
          ├── memory / Valkey lease
          ├── PostgreSQL request journal
          └── transactional outbox
                 │ signed webhook
                 ▼
             cmd/outboxd ──▶ money-platform / audit consumer

cmd/controld
          ├── local or PostgreSQL immutable snapshot store
          ├── Ed25519 signing
          └── publish / current / watch API
                 │ verified snapshot + LKG
                 ▼
             cmd/gatewayd

cmd/expert-brokerd
          ├── local atomic-file or PostgreSQL store
          ├── worker lease·renewal·fencing·retry
          ├── OpenAI Responses expert executor
          ├── ChatGPT web handoff
          └── tenant-scoped Remote MCP
```

## 구현된 핵심 경로

### Gateway와 프로토콜

- OpenAI Responses, OpenAI Chat Completions, Anthropic Messages, Gemini GenerateContent ingress
- canonical request·response·stream 모델과 strict loss 검증
- OpenAI, Anthropic, Google, DeepSeek, OpenRouter, OpenAI-compatible adapter
- 가상 모델 route와 선택적 `provider/model` 직접 호출; signed Control 사용 시 직접 호출은 기본 차단
- capability·region·cost·availability 기반 후보 필터
- health·queue·cost·affinity 기반 deterministic scoring
- 공급자 오류 taxonomy와 `Retry-After` 처리
- 첫 semantic event 전 retry, 이후 투명 retry 금지
- live SSE forwarding, terminal 검증, 중간 EOF 실패
- process-local cooldown과 memory 또는 Valkey Lua concurrency lease

### Signed Control distribution

- Ed25519 서명 snapshot의 원격 fetch·watch·검증
- key ID가 포함된 snapshot과 구키·신키 동시 신뢰를 통한 무중단 signing-key 회전
- 더 높은 revision만 원자 route swap; snapshot에 없는 implicit `local/echo` 경로 제거
- 동일 revision 내용 변경과 rollback 거절
- 원격 장애 시 검증된 last-known-good 사용
- snapshot 만료 시 readiness와 신규 요청 fail-closed
- signed snapshot의 현재 유효 price revision을 route swap과 함께 원자 적용해 request journal에 고정
- Control snapshot의 local atomic-file 또는 PostgreSQL immutable history
- 다중 Control replica 초기화·갱신 경쟁 복구
- secret-manager에서 signing seed/private key 주입

### Runtime journal과 outbox

- 사용자 request와 공급자 attempt의 분리 기록
- attempt별 provider·model·protocol·semantic commit·usage·error
- retry attempt를 포함한 total usage 집계
- request 완료와 outbox event의 단일 PostgreSQL transaction
- 동일 write 멱등성, 동일 ID·다른 payload conflict
- outbox lease, stale-worker fencing, bounded retry, dead letter
- HMAC signed webhook, replay-window verification, 명시적 duplicate acknowledgement, idempotency key
- reference webhook consumer와 outbox 운영 CLI

### Expert Escalation과 MCP

- 관련 파일 선별, symlink·path traversal 차단, digest, Git revision
- `.env`, API key, JWT, private key, DB URL, 고엔트로피 문자열 redaction
- consultation idempotency와 상태 머신
- local atomic-file 또는 PostgreSQL tenant/project-scoped persistence
- `FOR UPDATE SKIP LOCKED` claim, lease renewal, stale recovery, retry backoff
- worker fencing을 적용한 결과 원자 commit
- API 모델·reasoning 설정, web handoff, Remote MCP provenance 보존
- Codex용 MCP STDIO bridge
- Remote MCP의 `consultations:read`, `consultations:answer`, `consultations:admin` 분리와 발급 시 scope allowlist
- token의 tenant/project와 consultation·ContextPack scope 강제
- `argt2` key-ID token과 retiring HMAC key overlap; `argt1` legacy 검증의 명시적 종료 스위치

### 데스크톱과 운영 기반

- Wails v3 tray, single instance, close-to-tray, background autostart
- 로컬 Expert metadata와 immutable 32 KiB content-addressed ContextPack chunk의 분리 저장·검증·compaction
- Wails 앱과 headless 런타임에서 서버판과 동일한 provider composition 사용
- Windows Named Pipe와 Unix Domain Socket 로컬 IPC
- PostgreSQL migration runner: version, checksum, advisory lock, transaction
- 조직·프로젝트 bootstrap, virtual key 발급·폐기 CLI
- 비모호한 가상 키 형식 `zg_<environment>_<public-id>.<secret>`과 legacy parser
- 외부 인터페이스 bind 시 인증 강제
- PostgreSQL 18 integration test와 contract-generation release gate
- 40 KiB 저장소 파일 제한, 비정규 파일 누락 방지, 명시적 예외 정책, strict chunked manifest, deterministic source ZIP

## 실행물

| 실행물 | 책임 |
|---|---|
| 루트 Wails 앱 | 트레이, 설정, ContextPack 승인, 상담, 로컬 Gateway·IPC |
| `cmd/mcp-bridge` | MCP STDIO와 로컬 런타임 IPC 사이의 최소 bridge |
| `cmd/headless` | GUI 없는 로컬·CI용 런타임 |
| `cmd/gatewayd` | 호환 API ingress, provider router, streaming data plane, journal |
| `cmd/controld` | 서명 runtime snapshot publish·조회·watch |
| `cmd/controlctl` | 공개키·snapshot·모델 조회와 정책 publish |
| `cmd/expert-brokerd` | ContextPack·consultation API, worker, Remote MCP |
| `cmd/mcptokenctl` | tenant/project-scoped Remote MCP token 발급과 active key ID 표시 |
| `cmd/expertstorectl` | 로컬 Expert store 검증·통계·안전한 compaction |
| `cmd/releasepack` | 40 KiB 감사, chunked manifest 검증, deterministic source ZIP |
| `cmd/outboxd` | PostgreSQL outbox claim과 signed webhook delivery |
| `cmd/outboxctl` | outbox 상태·dead letter·requeue·purge 운영 |
| `cmd/webhook-sink` | 서명·멱등성 검증용 reference consumer |
| `cmd/dbmigrate` | embedded PostgreSQL migration up·down·status |
| `cmd/projectctl` | 조직·프로젝트 멱등 bootstrap·조회 |
| `cmd/keyctl` | scoped virtual key 발급·폐기 |

## 저장소 크기와 로컬 상태 운영

수작업 코드, 문서, 설정, 생성된 manifest 조각을 포함한 저장소 파일은 기본적으로 **40 KiB 이하**여야 한다. 예외가 정말 필요한 바이너리 fixture만 `config/file-size-exceptions.json`에 경로와 이유를 기록한다. 예외가 사라졌는데 설정만 남으면 감사도 실패한다.

```powershell
go run ./cmd/releasepack audit --root .
go run ./cmd/releasepack build --root . --output ../ai-runtime-gateway-source.zip
go run ./cmd/releasepack verify --root .
```

큰 `MANIFEST.json` 한 장 대신 루트 index와 `manifest/chunks/files-XXXX.json`으로 나눈다. 각 chunk도 40 KiB 제한을 통과하며 source ZIP은 정렬된 entry, 고정 timestamp, 원본 mode·size·SHA-256 재검증으로 재현 가능하게 만든다. symlink나 socket을 조용히 누락하지 않고 build를 실패시킨다.

로컬 Expert store는 consultation metadata와 source payload를 한 JSON에 넣지 않는다. source payload는 32 KiB 이하 SHA-256 chunk로 저장되고, metadata만 원자 교체한다. open 시 각 chunk의 전체 digest를 재검증하고 ContextPack read/write와 destructive compaction 사이의 chunk lifecycle 경쟁도 차단한다. 운영자는 다음 명령으로 손상 여부와 공간 사용량을 확인하고 오래된 terminal graph와 orphan chunk를 정리한다.

```powershell
go run ./cmd/expertstorectl verify
go run ./cmd/expertstorectl stats
go run ./cmd/expertstorectl compact --dry-run
go run ./cmd/expertstorectl compact --terminal-retention 720h --orphan-grace 24h
```

## 기준 버전

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

Wails v3는 alpha이므로 exact version을 유지한다. 도구 갱신은 protocol conformance, worker/outbox fencing, Wails lifecycle, MCP bridge 회귀 검증을 통과한 뒤 수행한다.

## 빠른 시작: 외부 인프라 없이

### 1. module path 변경

```powershell
./scripts/rename-module.ps1 -NewModule "github.com/<owner>/<repo>"
```

### 2. 의존성과 생성 코드 준비

```powershell
./scripts/bootstrap.ps1
go mod tidy
bun install
./scripts/generate.ps1
```

### 3. 로컬 Gateway 실행

공급자 key나 route를 설정하지 않으면 `local/echo`만 활성화된다.

```powershell
go run ./cmd/gatewayd
```

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri http://127.0.0.1:8080/v1/responses `
  -ContentType application/json `
  -Body '{"model":"local/echo","input":"Review the ledger","stream":false}'
```

### 4. 로컬 Expert Broker 실행

PostgreSQL URL이 없으면 `data/expert-broker/state.json`을 원자 저장소로 사용한다.

```powershell
go run ./cmd/expert-brokerd
```

`OPENAI_API_KEY`와 `EXPERT_MODEL`을 함께 설정하면 승인된 API consultation worker가 시작된다. 둘 중 하나라도 없으면 worker만 비활성화되고 web handoff와 수동 결과 반입은 계속 동작한다.

## 개발 스택

PostgreSQL·Valkey·migration·Control·Expert·Gateway를 함께 띄운다.

```powershell
docker compose -f deploy/docker-compose.dev.yml up --build
```

Signed outbox webhook 왕복까지 포함한다.

```powershell
docker compose -f deploy/docker-compose.dev.yml --profile outbox up --build
```

Compose의 고정 UUID·서명 seed·HMAC secret은 로컬 contract test 전용이다. production에서 재사용하지 않는다. 자세한 내용은 `docs/24-development-stack.md`를 따른다.

## 관리형 서버 초기화

### 1. migration과 project bootstrap

```powershell
$env:ARG_POSTGRES_URL = "postgres://airuntime:airuntime@127.0.0.1:5432/airuntime?sslmode=disable"
go run ./cmd/dbmigrate status
go run ./cmd/dbmigrate up

go run ./cmd/projectctl ensure `
  --organization-slug default `
  --organization-name "Default Organization" `
  --project-slug default `
  --project-name "Default Project"
```

### 2. virtual key 발급

```powershell
$pepper = New-Object byte[] 32
[Security.Cryptography.RandomNumberGenerator]::Fill($pepper)
$env:GATEWAY_VIRTUAL_KEY_PEPPER_B64 = [Convert]::ToBase64String($pepper)
$env:GATEWAY_KEY_ENVIRONMENT = "live"

go run ./cmd/keyctl issue `
  --project "<projectId>" `
  --tenant "<organizationId>" `
  --scopes "models:invoke" `
  --models "code-fast,code-deep" `
  --expires 720h
```

평문 key는 발급 시 한 번만 반환된다. PostgreSQL에는 peppered HMAC만 저장한다. pepper와 평문 key를 같은 secret store나 log에 남기지 않는다.

### 3. Control snapshot

단일 process는 local store를 사용할 수 있다. replica를 둘 때는 PostgreSQL store와 공용 signing secret을 사용한다.

```powershell
$env:CONTROL_SNAPSHOT_STORE = "postgres"
$env:CONTROL_POSTGRES_URL = $env:ARG_POSTGRES_URL
$env:CONTROL_SIGNING_PRIVATE_KEY = "<base64url-ed25519-seed>"
$env:CONTROL_BEARER_TOKEN = "<operator-token>"
go run ./cmd/controld
```

공개키 확인과 정책 publish:

```powershell
go run ./cmd/controlctl signing-key --raw
go run ./cmd/controlctl publish --file config/runtime-snapshot.example.json
```

Gateway에는 공개키만 제공한다. Control 연결이 설정되면 `provider/model` 직접 호출은 기본적으로 꺼지고, snapshot에 서명된 virtual model만 호출할 수 있다.

```powershell
$env:GATEWAY_CONTROL_URL = "http://127.0.0.1:8081"
$env:GATEWAY_CONTROL_BEARER_TOKEN = "<operator-token>"
$env:GATEWAY_CONTROL_SIGNING_PUBLIC_KEY_B64 = "<public-key>"
$env:GATEWAY_CONTROL_REQUIRED = "true"
```

### 4. Remote MCP scoped token

```powershell
$env:EXPERT_MCP_TOKEN_SECRET = "<at-least-32-random-bytes>"

go run ./cmd/mcptokenctl issue `
  --subject chatgpt-reviewer `
  --tenant "<organizationId>" `
  --project "<projectId>" `
  --scopes "consultations:read,consultations:answer" `
  --ttl 1h
```

Scoped token secret이 설정되면 Broker 관리 bearer는 MCP token으로 자동 재사용되지 않는다. read·answer token은 tenant와 project를 모두 요구하며, cluster-wide `consultations:admin`은 다른 scope와 섞어 발급할 수 없다. `EXPERT_MCP_ALLOW_BROKER_TOKEN=true`는 로컬 호환 외에는 사용하지 않는다.

### 5. 실제 provider와 Valkey

`config/gateway-routes.example.json`의 placeholder model ID를 실제 upstream ID로 바꾸거나 signed Control snapshot을 사용한다.

```powershell
$env:OPENAI_API_KEY = "..."
$env:ANTHROPIC_API_KEY = "..."
$env:GATEWAY_VALKEY_URL = "redis://127.0.0.1:6379/0"
$env:GATEWAY_VALKEY_PREFIX = "arg"
go run ./cmd/gatewayd
```

동일 account pool을 여러 Gateway가 공유할 때 memory lease와 Valkey lease를 섞으면 전역 동시성 상한이 깨진다.

### 6. Runtime journal과 outbox

```powershell
$env:GATEWAY_RUNTIME_JOURNAL = "postgres"
$env:GATEWAY_POSTGRES_URL = $env:ARG_POSTGRES_URL
$env:GATEWAY_PRICE_REVISION_ID = "price-2026-08"
# Operator bearer 또는 인증 없는 loopback 호출도 journal에 넣는다면 아래 감사 식별자가 필요하다.
$env:GATEWAY_OPERATOR_TENANT_ID = "<organizationId>"
$env:GATEWAY_OPERATOR_PROJECT_ID = "<projectId>"
$env:GATEWAY_OPERATOR_VIRTUAL_KEY_ID = "<operatorVirtualKeyId>"
# Project ID와 virtual-key ID는 canonical UUID여야 하며 잘못된 값은 gateway 시작 시 거절된다.

$env:OUTBOX_POSTGRES_URL = $env:ARG_POSTGRES_URL
$env:OUTBOX_WEBHOOK_URL = "https://money.example.internal/events"
$env:OUTBOX_WEBHOOK_SECRET = "<random-secret>"
go run ./cmd/outboxd
```

Gateway journal의 usage event는 고객 잔액 원장이 아니다. downstream money-platform이 signature와 event idempotency를 검증한 뒤 quote·hold·capture·release를 소유해야 한다.

## 인증 모드

| 설정 | 동작 |
|---|---|
| 아무 인증 변수도 없음 | loopback 개발 모드만 허용 |
| `GATEWAY_BEARER_TOKEN` | 운영자 bearer 인증 |
| virtual-key pepper + PostgreSQL | project·scope·model 제한 virtual key 인증 |
| bearer와 virtual key 모두 설정 | 둘 중 하나가 유효하면 통과 |
| scoped MCP secret | tenant/project/scope 제한 Remote MCP 인증 |
| explicit legacy MCP bearer | cluster-wide 호환 credential |

Gateway, Control, Expert Broker, Outbox 관리 endpoint는 loopback이 아닌 주소에 bind할 때 인증이 없으면 시작을 거부한다.

## 테스트와 검증

형식 수정과 검증을 분리한다. `check`는 소스를 자동 변경하지 않는다.

```powershell
./scripts/format.ps1
./scripts/check.ps1
```

PostgreSQL integration test는 지정 DB를 truncate하므로 전용 일회성 DB에서만 실행한다.

```powershell
$env:ARG_TEST_POSTGRES_URL = "postgres://airuntime:airuntime@127.0.0.1:5432/airuntime_test?sslmode=disable"
$env:ARG_TEST_POSTGRES_ALLOW_RESET = "I_UNDERSTAND_THIS_DATABASE_WILL_BE_TRUNCATED"
go test -tags=integration -count=1 ./tests/postgres
```

CI와 release는 Go test·race·vet, Buf·sqlc contract generation, frontend check, PostgreSQL durable-delivery test를 gate로 사용한다. 이 샌드박스에서 실제로 수행한 범위는 `VALIDATION.md`를 따른다.

## 아직 남은 production 작업

- money-platform의 quote–hold–capture client와 usage reconciliation
- provider별 주기 probe, 실제 TTFT·TPS 기반 분산 health 공유
- OS Credential Manager·Keychain·Secret Service adapter
- Remote MCP OIDC/OAuth, key ID 기반 secret rotation
- Control signing key rotation과 dual-trust rollout
- Control Console의 인증·조직·key·route·audit·usage 관리
- Wails Windows/macOS/Linux 실기기 lifecycle, installer, signing, rollback
- 실제 공급자 fixture·청구 usage 대사·24시간 이상 soak test
- PostgreSQL partitioning과 retention을 실제 부하 기준으로 확정

## 핵심 금지선

- MCP STDIO process는 stdout에 log를 쓰지 않는다.
- 첫 semantic event 이후 다른 provider로 투명 retry하지 않는다.
- upstream EOF를 임의 completed event로 바꾸지 않는다.
- Valkey를 잔액·상담 결과·usage 원본의 권위 저장소로 쓰지 않는다.
- ChatGPT 웹 DOM 자동조작이나 session cookie 추출을 제품 기능으로 넣지 않는다.
- provider 고유 reasoning·tool metadata를 조용히 버리지 않는다.
- Wails adapter에 protocol·routing·money 도메인 로직을 넣지 않는다.
- Control snapshot row와 applied migration을 수정해 이력을 재작성하지 않는다.
- generated binding, protobuf, sqlc code를 수동 편집하지 않는다.

## 문서 입구

- `IMPLEMENTATION_STATUS.md`
- `docs/02-system-context.md`
- `docs/05-expert-escalation.md`
- `docs/06-mcp-contract.md`
- `docs/08-protocol-compiler.md`
- `docs/09-routing.md`
- `docs/10-accounting.md`
- `docs/11-security-threat-model.md`
- `docs/15-build-and-release.md`
- `docs/20-operations-runbook.md`
- `docs/21-control-snapshot-distribution.md`
- `docs/22-runtime-journal-and-outbox.md`
- `docs/23-remote-mcp-security.md`
- `docs/24-development-stack.md`
- `VALIDATION.md`
- `TREE.md`
