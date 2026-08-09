# 16. Implementation Order

이 문서는 기능 목록이 아니라 의존 순서와 완료 판정 기준을 고정한다. 현재 저장소는 Stage 0, Stage 1, Stage 2의 핵심 코드까지 구현했다. 남은 일은 새로운 기능을 더 붙이는 작업보다 실제 인프라·공급자·데스크톱 환경에서 운영 조건을 검증하고 money-platform 경계를 연결하는 작업이다.

## Stage 0A: 계약 — 완료

완료된 범위:

- canonical request·response·stream type
- strict/compatible/passthrough loss taxonomy
- provider error taxonomy
- consultation 상태 전이와 idempotency
- ContextPack manifest와 content digest
- MCP tool 입출력 schema
- PostgreSQL schema와 embedded migration 계약
- virtual key v2 형식과 legacy parser

완료 판정:

- 외부 모듈 없이 실행 가능한 protocol·state·storage 테스트 통과
- migration 파일 version·checksum·up/down pairing 검사 통과
- 알 수 없는 provider event의 raw 보존

## Stage 0B: 로컬 런타임 — 코드 완료, 실기기 검증 대기

구현 범위:

- Wails v3 tray와 single instance
- close-to-tray와 `--background` 로그인 자동 시작
- MCP STDIO bridge → Named Pipe/Unix Domain Socket
- ContextPack preview와 consultation 생성·조회
- local atomic state persistence
- 로컬 Gateway 시작·중지

운영 완료 조건:

- Windows 일반 사용자·관리자·다른 사용자 계정에서 Named Pipe ACL 검증
- 절전·로그아웃·재로그인·업데이트 뒤 autostart 검증
- macOS launch lifecycle과 Linux desktop session 검증
- MCP stdout 오염, 런타임 crash, 재접속 회귀 테스트

## Stage 1A: Gateway data plane — 코드 완료

구현 범위:

- OpenAI Responses·Chat, Anthropic Messages, Gemini ingress
- OpenAI·Anthropic·Google·DeepSeek·OpenRouter·OpenAI-compatible adapter
- virtual model route와 direct `provider/model`
- live SSE forwarding
- cancellation, response-size limit, terminal validation
- provider error 분류와 Retry-After
- pre-semantic retry, post-semantic fail-visible
- process-local health degradation과 cooldown
- memory/Valkey concurrency lease

운영 완료 조건:

- 각 공급자의 실제 key로 non-stream·stream·tool·reasoning conformance 통과
- 429, quota exhaustion, overloaded, timeout, transport reset fixture 통과
- provider usage와 local usage estimate의 허용 오차 확정
- Valkey failover 중 lease 상한 위반 여부 검증
- 24시간 soak test에서 goroutine·connection·memory leak 부재 확인

## Stage 1B: Expert Broker — 코드 완료

구현 범위:

- ContextPack preview·build·redaction
- local atomic file과 PostgreSQL repository
- consultation create·approve·cancel·list·get
- worker claim·renew·retry·stale recovery·maximum attempts
- worker fencing을 적용한 result commit
- OpenAI Responses expert executor
- ChatGPT web handoff와 수동 result import
- result model attestation
- Remote MCP get·submit

운영 완료 조건:

- 실제 PostgreSQL에서 다중 worker contention·crash recovery 테스트
- API model usage와 consultation maximum cost reconciliation
- scoped Remote MCP의 tenant/project 격리 실서버 conformance
- OIDC/OAuth와 key ID rotation 설계
- stale ContextPack 결과의 UI 경고와 재검토 절차

## Stage 1C: Control snapshot — 코드 완료

구현 범위:

- Ed25519 signing key load/create와 secret-manager 주입
- local atomic store와 PostgreSQL immutable history
- current·publish·watch·public-key API
- 다중 replica 초기화·갱신 경쟁 복구
- Gateway fetch·watch·signature 검증·atomic route swap
- last-known-good persistence와 snapshot expiry fail-closed

운영 완료 조건:

- signing key rotation과 key ID 기반 dual trust
- Control DB failover·watch 장기 soak
- route file과 signed Control snapshot의 production SSOT 고정
- snapshot TTL과 장애 복구 목표 확정

## Stage 2: 관리형 운영 기반 — 핵심 코드 완료

완료된 범위:

- embedded PostgreSQL migration runner
- organization/project bootstrap CLI
- PostgreSQL virtual key issue·authenticate·revoke
- project scope와 model allowlist
- Valkey distributed provider lease
- runtime request·provider attempt·usage journal
- transactional PostgreSQL outbox
- worker lease·fencing·dead letter·signed webhook
- PostgreSQL integration CI contract

다음 작업:

- provider health probe와 분산 측정값
- audit event와 관리자 RBAC
- provider credential KMS/OS keychain adapter
- runtime retention·partition 정책

## Stage 3: Accounting과 상용화 — 계약만 구현

현재 quote → hold → capture → release·adjustment의 domain contract와 idempotency 테스트가 있다. 실제 결제와 mandarin 차감은 다음 조건 뒤에 연다.

- `zdp-money-platform` ConnectRPC 계약
- request charge와 provider attempt cost 분리
- stream partial failure 비용 정책
- 늦게 도착한 usage adjustment
- 환불·취소·무료/유료 크레딧 소진 revision
- 구현된 signed PostgreSQL outbox와 money-platform consumer idempotency의 E2E 검증
- provider invoice reconciliation

결제부터 열고 usage persistence를 나중에 붙이는 순서는 금지한다.

## Stage 4: Release certification

출시 후보는 다음 증거가 모두 있어야 한다.

- clean machine build와 재현 가능한 artifact hash
- Wails installer upgrade·rollback 실기기 기록
- PostgreSQL backup·PITR·restore rehearsal
- Valkey 장애와 provider outage game day
- MCP client별 conformance report
- 실제 공급자 bill reconciliation report
- security review: SSRF, secret redaction, local IPC ACL, token scope
- 24시간 soak와 graceful shutdown report
