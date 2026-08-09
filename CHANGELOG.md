# Changelog

## 0.5.1-dev — ssealed lifecycle and portable source modes

- RelayDock를 ssealed의 minimal monorepo profile과 desktop-app·cli-tool addon에 연결했다.
- 공개 Go module path와 내부 import를 `github.com/0disoft/relaydock`로 확정했다.
- desktop binary, source archive, workspace package, Wails product metadata와 public protocol identity를 RelayDock 이름으로 통일했다.
- 신규 local state·IPC·autostart 식별자를 RelayDock로 전환하고 기존 state directory는 읽기 호환으로 보존했다.
- release workflow 앞단에 version·module·lockfile·license·source manifest fail-closed readiness gate를 추가했다.
- generated contracts·source·server·web·Windows desktop 산출물의 체크섬에 job-scoped SLSA build provenance attestation을 추가했다.
- pinned Syft로 각 release artifact 집합의 SPDX JSON을 만들고 같은 checksum subject에 SBOM attestation을 연결했다.
- 여섯 service의 unpublished Linux/amd64 OCI release-candidate와 image digest·archive checksum·SBOM·attestation artifact를 추가했다.
- CI와 release의 Go module resolution을 read-only로, Bun 설치를 루트 workspace `bun.lock` 기반 frozen install로 통일했다.
- 모든 service Dockerfile이 `go.sum`을 필수 입력으로 사용하고 read-only module resolution과 non-root runtime을 유지하도록 고정했다.
- service image의 Go buildinfo와 OCI labels에 동일한 version·commit·build time release envelope를 강제했다.
- 생성된 제품·아키텍처·운영 문서는 기존 번호 문서를 가리키는 얇은 라우팅 인덱스로 유지한다.
- Windows source scan에서 일반 파일을 `0644`, 셸 스크립트를 `0755`로 정규화해 POSIX manifest mode의 불필요한 전체 변경을 막았다.
- source archive의 mode 검증도 같은 정규화 계약을 사용하고 Windows·POSIX 회귀 테스트를 추가했다.

## 0.5.0-dev — bounded modules, chunked local state, and key rotation

저장소와 로컬 상태가 커질수록 생기던 단일 파일 병목을 제거하고, 서명키·MCP HMAC key를 중단 없이 회전할 수 있게 만들었다.

### Bounded repository and modularization

- 저장소 regular file의 기본 상한을 40 KiB로 고정하고 명시적 path·reason 예외만 허용한다.
- 거대 `MANIFEST.json`을 root index와 30 KiB 목표 chunk로 분리했다.
- manifest chunk path·범위·정렬·SHA-256·aggregate hash·실제 파일 일치를 검증한다.
- 정렬된 entry와 정규화 timestamp를 쓰는 deterministic source ZIP builder를 추가했다.
- archive 생성 중 source size·mode·hash가 바뀌면 실패하고, repository 내부 output path를 거절한다.
- symlink·socket·device 같은 비정규 파일을 조용히 누락하지 않고 source release 단계에서 명시적으로 거절한다.
- manifest와 size-policy JSON은 trailing value까지 거절하는 strict decoder로 검증한다.
- unique temporary file과 Windows rename 제약을 처리하는 backup swap publish를 적용했다.
- CandidateSource, HTTP Gateway, HTTP provider adapter, runtime gateway, local Expert store를 책임별 파일로 분리했다.

### Local Expert store v3

- ContextPack source를 metadata JSON에서 분리하고 최대 32 KiB의 immutable SHA-256 chunk로 저장한다.
- ContextPack과 consultation을 하나의 metadata transaction으로 생성한다.
- v1·v2 embedded payload store를 v3 chunk store로 migration하고 원본 `.v2.bak`을 보존한다.
- chunk 누락·크기뿐 아니라 실제 SHA-256 payload를 store open 시 전수 검증한다.
- ContextPack write/read와 chunk garbage collection을 RW 경계로 직렬화해 동시 compaction이 live chunk를 지우는 경쟁을 막는다.
- immutable ContextPack ID 충돌을 검증하고 terminal retention, orphan graph 정리, chunk garbage collection, dry-run과 stats를 구현했다.
- `expertstorectl`로 local store verify·stats·compact를 수행한다.
- PostgreSQL store도 ContextPack과 consultation 생성에 하나의 transaction을 사용한다.

### Signing and token rotation

- Control snapshot에 `signingKeyId`를 추가하고 active Ed25519 key와 retiring public key를 동시에 신뢰한다.
- 구키로 서명된 persisted snapshot을 검증한 뒤 active key의 다음 revision으로 재발행한다.
- Gateway와 Control 모두 legacy no-key-ID snapshot 종료 스위치를 제공한다.
- Remote MCP token을 `argt2.<key-id>.<claims>.<hmac>` 형식으로 변경했다.
- active HMAC key로만 발급하고 retiring key ring으로 기존 token을 검증한다.
- key count·secret size·subject·scope·identity·payload 크기를 제한하고, 공백 정규화 뒤 중복되는 key ID를 거절한다.
- Ed25519·HMAC key ID를 64자 이내 안전한 문자 집합으로 제한하고 trusted-key JSON의 trailing value를 거절한다.
- `argt1` 호환 검증을 별도 스위치로 두고 최대 token 수명 이후 제거할 수 있게 했다.
- active key ID에 다른 secret을 덮어쓰는 구성을 거절한다.

### Release and CI

- `releasepack audit`, `verify`, `build` 명령과 Task·check script·CI gate를 연결했다.
- release workflow가 deterministic source archive와 SHA-256을 별도 artifact로 만든다.
- server artifact에 `expertstorectl`과 `releasepack`을 추가했다.
- file-size policy, symlink 누락 방지, strict JSON, manifest tamper, deterministic archive, replacement publish, local chunk tamper·concurrent compaction, key rotation·token bound 회귀 테스트를 추가했다.

## 0.4.0-dev — signed control, durable usage, and scoped MCP

운영에서 가장 위험했던 Control·usage·Remote MCP 경계를 실제 실행 경로로 연결했다.

### Signed Control distribution

- Gateway가 Control Plane의 Ed25519 서명 snapshot을 검증하고 route table에 원자 적용한다.
- 원격 Control 장애 시 검증된 last-known-good를 사용하되 snapshot 만료 뒤에는 readiness와 신규 요청을 fail-closed 처리한다.
- 동일 revision의 내용 변경, rollback, 잘못된 signature를 거절한다.
- Control snapshot을 로컬 원자 파일 또는 PostgreSQL immutable history에 저장할 수 있다.
- PostgreSQL publisher를 serializable transaction과 advisory lock으로 직렬화했다.
- 여러 Control replica가 초기화·갱신 경쟁을 일으켜도 최신 committed revision을 다시 읽고 정상 기동한다.
- Ed25519 seed/private key를 secret manager 환경 변수로 주입할 수 있다.
- `controlctl`로 공개키 조회, snapshot 저장, 모델 조회, 정책 publish를 수행한다.

### Runtime journal and outbox

- 사용자 request와 provider attempt를 분리한 PostgreSQL journal을 Gateway hot path에 연결했다.
- retry attempt별 provider, model, protocol, semantic commit, token usage, 오류를 기록한다.
- request 완료와 outbox event 생성을 하나의 transaction으로 묶었다.
- 동일 ID의 동일 쓰기는 멱등 처리하고, 같은 ID의 다른 내용은 conflict로 거절한다.
- `outboxd`에 batch claim, bounded concurrency, lease-expiry fencing, stale-worker rejection, Retry-After, exponential backoff, dead letter를 추가했다.
- 완료·retry·dead-letter·release가 처리 완료 시각과 lease expiry를 함께 검사한다.
- webhook payload에 event idempotency key와 HMAC signature를 붙이고 redirect·원격 평문 HTTP를 기본 거절한다.
- reference `webhook-sink`가 서명, replay window, 동일 payload 재전송, payload conflict를 검증한다.
- `outboxctl`로 상태 조회, dead-letter requeue, published event purge를 수행한다.

### Remote MCP security

- HMAC scoped token에 audience, token ID, subject, tenant, project, scope, 발급·만료 시각을 넣었다.
- `consultations:read`, `consultations:answer`, `consultations:admin`을 분리했다.
- scoped token의 tenant/project가 consultation 및 ContextPack과 모두 일치해야 한다.
- scoped token secret이 설정된 경우 Broker 관리 bearer를 MCP credential로 자동 재사용하지 않는다.
- legacy static token은 명시 설정으로만 유지하고 multi-tenant 기본 경로에서 제외했다.
- malformed boolean과 accidental broker-token reuse를 막는 독립 설정 테스트를 추가했다.

### Policy and identity hardening

- signed snapshot을 authoritative route table로 취급해 문서에 없는 implicit `local/echo` 경로를 제거한다.
- snapshot의 현재 유효 price revision을 route와 원자 교체하고 runtime journal이 이를 요청 시작 시점에 고정한다.
- Control 연결 시 `provider/model` 직접 라우팅을 기본 차단하고, process-local candidate 상태를 TTL·상한으로 제한한다.
- PostgreSQL journal에서 operator bearer의 임시 문자열 ID를 구성된 tenant/project/key UUID로 치환한다.
- virtual-key 인증이 없는 PostgreSQL journal은 operator audit identity가 없으면 시작을 거절한다.
- operator project/key ID를 공통 canonical UUID validator로 시작 시 검증해 첫 요청에서 PostgreSQL cast 오류가 발생하는 구성을 차단한다.
- scoped token의 canonical base64url을 강제하고 crypto random ID 생성 실패를 약한 시간 기반 ID로 대체하지 않는다.
- `mcptokenctl`은 Remote MCP scope allowlist, tenant/project pair, standalone admin 규칙을 검증한다.
- webhook의 일반 409는 성공으로 간주하지 않고, 동일 event ID를 명시한 duplicate acknowledgement만 수락한다.
- 사용자 지정 HTTP client를 제공해도 webhook redirect를 항상 차단한다.

### HA, CI, and development stack

- Control PostgreSQL snapshot store의 polling watch와 transient read recovery를 구현했다.
- PostgreSQL 18 integration test가 migration, signed snapshot, request→attempt→usage→outbox, idempotency, lease fencing과 reclaim을 검증한다.
- CI와 release workflow가 Buf·sqlc 생성 코드 compile 및 PostgreSQL integration gate를 통과해야 artifact를 만든다.
- Windows runner에서 Wails v3 desktop과 MCP bridge를 실제 compile하고 checksum이 있는 portable ZIP을 생성한다.
- 개발 Compose에 migration, deterministic seed, PostgreSQL Control store, Gateway journal, optional signed outbox webhook profile을 연결했다.
- server·operations release artifact에 `outboxd`, `controlctl`, `mcptokenctl`, `outboxctl`, `webhook-sink`를 포함했다.
- schema snapshot과 sqlc query를 최종 migration 상태에 맞췄다.

## 0.3.0-dev — durable gateway and expert runtime

메모리 중심 수직 슬라이스를 영속 저장소와 실제 provider composition을 갖춘 reference implementation으로 확장했다.

### Gateway runtime

- 환경 기반 OpenAI·Anthropic·Google·DeepSeek·OpenRouter·OpenAI-compatible provider 등록을 추가했다.
- virtual model route와 `provider/model` 직접 라우팅을 연결했다.
- 공급자 오류를 invalid request, authentication, permission, quota, rate limit, overloaded, timeout, transport, upstream으로 분류한다.
- `Retry-After`와 첫 semantic event 경계를 반영한 안전한 retry를 구현했다.
- 공급자별 health degradation, exponential cooldown, success recovery를 추가했다.
- unavailable candidate가 선택될 수 있던 router 결함을 수정했다.
- response 완료 뒤 재생하던 stream을 실시간 SSE delta forwarding으로 교체했다.
- memory lease 외에 Valkey Lua 기반 acquire·renew·release와 server-time expiry를 추가했다.

### Expert durability

- ContextPack·consultation·result·idempotency를 하나의 원자 JSON 상태로 저장하는 local store를 추가했다.
- PostgreSQL tenant/project-scoped ContextPack·consultation·result repository를 추가했다.
- `FOR UPDATE SKIP LOCKED` worker claim, lease renewal, stale recovery, retry backoff, maximum attempts를 구현했다.
- worker fencing을 결과 commit에 강제해 lease를 잃은 worker의 늦은 완료를 거절한다.
- 기본 worker ID를 hostname·PID·random ID 조합으로 만들어 인스턴스 충돌을 제거했다.
- API model·reasoning 설정, web handoff 선언, Remote MCP 제출자의 model attestation을 결과와 함께 보존한다.

### Control and authentication

- Control snapshot과 Ed25519 signing key를 원자 파일로 영속화했다.
- PostgreSQL virtual key 발급·검증·폐기를 구현했다.
- 가상 키 형식을 `zg_<environment>_<public-id>.<secret>`으로 변경하고 legacy key parser를 유지했다.
- project scope, model allowlist, `models:invoke` scope를 Gateway ingress에 강제한다.
- 조직·프로젝트 멱등 bootstrap CLI와 virtual key 관리 CLI를 추가했다.

### Database and operations

- embedded migration runner에 version ordering, SHA-256 checksum, advisory lock, transaction, up/down/status를 추가했다.
- consultation worker lease, result attestation, context payload를 위한 `000002_expert_durability` migration을 추가했다.
- Gateway route 예시, 확장된 환경 변수, 운영 runbook을 추가했다.

### Verification

- retry, live stream, provider error, router availability, worker recovery·fencing, local durability, control snapshot, virtual key parser, migration, Valkey lease 계약 테스트를 추가했다.
- 외부 모듈이 필요 없는 core 범위는 test·race·vet 대상으로 유지한다.

## 0.1.0-dev — implemented vertical slice

초기 디렉터리·인터페이스 골격을 실행 가능한 개발용 수직 슬라이스로 확장했다.

### Runtime and protocol

- OpenAI Responses, OpenAI Chat Completions, Anthropic Messages, Gemini GenerateContent 변환기를 추가했다.
- canonical request·response·stream 모델과 변환 손실 보고를 구현했다.
- 명시적 terminal event가 없는 SSE/NDJSON EOF를 실패로 처리한다.
- 첫 semantic event 이후 투명 provider retry를 금지한다.
- 개발용 `local/echo` Gateway가 JSON과 SSE 요청을 왕복한다.

### Expert escalation

- ContextPack 파일 선별, digest, 경로 탈출 방지, secret redaction을 구현했다.
- consultation 상태 머신, idempotency, 취소, 결과 저장을 구현했다.
- OpenAI Responses 전문가 실행 경로와 ChatGPT 웹 handoff/result import를 추가했다.
- 최대 비용 상한과 delegation depth 제한을 추가했다.

### Desktop and MCP

- Wails v3 single instance, tray, close-to-tray, `--background` autostart를 연결했다.
- Codex용 MCP STDIO 브리지와 로컬 Named Pipe/Unix Domain Socket 런타임을 구현했다.
- Wails 화면에서 ContextPack 미리보기, 상담 생성, API 전문가 실행, 웹 결과 반입을 수행한다.
- 현재 OS용 desktop·MCP bridge portable package 작업을 추가했다.
