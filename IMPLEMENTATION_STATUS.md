# Implementation Status

## 현재 단계

`0.5.1-dev`는 운영 경로와 bounded-file 계약을 가진 reference implementation이다. 외부 인프라가 없으면 `local/echo`, 원자 JSON 저장소, memory lease로 실행되고, PostgreSQL·Valkey를 연결하면 영속 consultation, virtual key, signed Control snapshot, runtime journal, transactional outbox와 분산 provider lease가 활성화된다.

완전한 production-ready 판정은 아니다. 실제 공급자 계정, money-platform, Go 1.26 전체 dependency build, PostgreSQL·Valkey 실서버, Wails installer·update·code signing은 대상 환경에서 별도 통과해야 한다.

## 구성요소별 상태

| 영역 | 상태 | 현재 구현 | 남은 production 작업 |
|---|---|---|---|
| Canonical protocol | 구현·테스트 | OpenAI Responses·Chat, Anthropic Messages, Gemini decode/encode, strict loss report | 실제 provider fixture 확대와 version drift 감시 |
| Live streaming | 구현·테스트 | 즉시 delta 전달, semantic commit, tool delta, terminal 검증, 중간 EOF 실패 | provider 공식 resume token은 별도 adapter 필요 |
| Gateway composition | 구현·테스트 | 환경 provider 등록, virtual route, 정책으로 제한되는 `provider/model`, signed snapshot authoritative route·price revision swap | 대규모 route benchmark와 hot-reload soak |
| Provider adapters | 구현 | OpenAI, Anthropic, Google, DeepSeek, OpenRouter, generic OpenAI-compatible | 실제 계정 conformance·billing usage 대사 |
| Error taxonomy | 구현·테스트 | auth, permission, quota, rate-limit, overloaded, timeout, transport, invalid request | provider별 세부 error fixture |
| Retry safety | 구현·테스트 | 첫 semantic event 전만 retry, 이후 fail-visible | resume 지원 provider의 명시적 continuation |
| Routing | 구현·테스트 | capability·region·cost·availability filter, deterministic score | 실측 TTFT·TPS와 분산 health 공유 |
| Provider cooldown | 구현·테스트 | 오류 유형별 health degradation, exponential cooldown, success recovery | Valkey/Control 기반 multi-instance health sharing |
| Concurrency lease | 구현·테스트 | memory lease, Valkey Lua acquire·renew·release, server time | 실제 Valkey cluster·Sentinel·partition 검증 |
| Signed Control snapshot | 구현·테스트 | key-ID Ed25519 fetch·watch·verify, dual trust, old-key re-sign, atomic route swap, LKG, expiry fail-closed | 장기 watch·실제 secret-manager rotation soak |
| Control local store | 구현·테스트 | atomic snapshot/key persistence | single-instance 용도로만 유지 |
| Control PostgreSQL store | 구현 | immutable revision history, advisory lock, polling watch, HA init recovery | 실제 multi-replica contention·DB failover 검증 |
| Runtime request journal | 구현·테스트 | request·attempt·commit·usage·error lifecycle, retry total usage, operator UUID startup validation | table partition·retention·실부하 tuning |
| Transactional outbox | 구현·테스트 | FinishRequest와 event 단일 transaction, idempotency conflict | downstream money-platform E2E |
| Outbox worker | 구현·테스트 | lease, stale-worker fencing, Retry-After, backoff, dead letter, graceful stop | 실제 network partition·receiver outage soak |
| Signed webhook | 구현·테스트 | HMAC, replay window, idempotency key, redirect/insecure HTTP guard | mTLS 또는 workload identity |
| Reference webhook sink | 구현·테스트 | signature·duplicate·payload conflict contract | production ledger로 사용 금지 |
| ContextPack | 구현·테스트 | file selection, path defense, symlink exclusion, digest, Git revision, redaction, limit | symbol index, tokenizer-aware estimate, R2 adapter |
| Consultation local store | 구현·테스트 | v3 metadata, immutable 32 KiB content chunks, full digest open verification, atomic create, chunk lifecycle RW lock, migration, compaction·stats | 실제 대형 저장소 crash·disk-full·antivirus soak |
| Consultation PostgreSQL | 구현 | tenant/project scope, idempotency, claim, lease, stale recovery, fencing | 실제 long contention·backup restore |
| Expert worker | 구현·테스트 | concurrency, renew, retry, max attempts, fencing | cost authorization·multi-provider executor |
| Expert API route | 구현 | OpenAI Responses, reasoning mode·effort, structured result validation | 실제 model conformance·usage reconciliation |
| Web handoff | 구현 | read token, expiry, result import, user-declared attestation | ChatGPT connector UX와 organization policy |
| Remote MCP | 구현·부분 테스트 | host·origin·body guard, bounded argt2 key-ID HMAC token, retiring key overlap, normalized key collision guard, tenant/project isolation | OIDC/OAuth, live SDK conformance |
| MCP bridge | 구현 | STDIO tools → local IPC | Codex·Claude Code·OpenCode 실사용 conformance |
| Local IPC | 구현·테스트 | Unix socket, Windows named pipe, frame limit, reconnect | Windows multi-user ACL 실기기 검증 |
| Wails desktop | 구현 | tray, single instance, close-to-tray, autostart, settings, provider runtime | installer·signing·sleep/resume·updater rollback |
| Virtual key | 구현·테스트 | PostgreSQL HMAC key, scope·model auth, revoke, v2·legacy parser | pepper rotation·audit·high-volume cache |
| Database migration | 구현·테스트 | embedded SQL, checksum, advisory lock, per-migration transaction | actual upgrade matrix and PITR exercise |
| Bootstrap CLI | 구현 | organization/project ensure, key issue/revoke, Control/MCP/outbox ops | Control Console RBAC 연결 |
| Accounting domain | 개발 계약 | quote, hold, capture, release, adjustment, idempotency | mandarin money-platform client·reconciliation |
| Storage primitives | 부분 구현 | encrypted memory secret, credential, expiring object, durable outbox | OS keychain, KMS, R2/S3 |
| Updater | staging 구현 | manifest signature, SHA-256, size, pending artifact | platform bootstrap replacement·rollback |
| Control Console | 읽기 화면 | health·snapshot·model route 조회 | login, organization, key, route, audit, usage UI |
| Packaging | 구현·테스트 | desktop+MCP bundle, server/ops list, 40 KiB audit, non-regular-file rejection, strict chunked manifest, deterministic source ZIP | native installer·artifact signing·cross-Go-version reproducibility |
| CI contracts | 구현 | Go, Buf, sqlc, frontend, PostgreSQL integration gates | 실제 hosted runner 통과·artifact 서명 |

## 0.5.1-dev까지 닫힌 주요 공백

1. 40 KiB 저장소 파일 상한과 이유가 필요한 예외 정책을 CI·release에 강제했다.
2. 단일 대형 manifest를 root index와 검증 가능한 chunk로 분리했다.
3. source ZIP을 저장소 밖에서 deterministic하게 만들고 생성 중 파일 교체를 탐지한다.
4. CandidateSource, HTTP ingress, provider stream decoder, runtime attempt·lease를 책임별 파일로 분리했다.
5. 로컬 ContextPack source를 metadata에서 떼어 32 KiB content-addressed chunk로 저장한다.
6. legacy local store migration, immutable pack conflict, missing chunk 검증과 orphan garbage collection을 추가했다.
7. local과 PostgreSQL consultation 생성에서 ContextPack과 consultation의 원자성을 강화했다.
8. Control snapshot에 signing key ID와 old/new public-key overlap을 추가했다.
9. Remote MCP token을 key-ID argt2 형식으로 바꾸고 retiring HMAC key 검증, key-map collision 방지와 claims 크기 상한을 추가했다.
10. legacy no-key-ID snapshot과 argt1 token을 명시적으로 종료할 수 있게 했다.
11. `expertstorectl`과 `releasepack` 운영 CLI를 release artifact와 문서에 포함했다.
12. manifest tamper, archive replacement, store migration·compaction, key rotation 회귀 테스트를 추가했다.

## 외부 의존성 없이 현재 검증된 범위

- protocol canonical roundtrip과 strict loss
- unknown provider event 보존
- parallel tool-call delta ordering과 terminal 검증
- pre-semantic retry와 post-semantic retry 차단
- provider error taxonomy와 cooldown
- signed snapshot validation, LKG manager, concurrent Control initialization recovery
- ContextPack secret 제거·digest·path defense
- consultation lifecycle·idempotency·worker fencing
- argt2 scoped MCP token 발급·검증, key rotation, argt1 migration과 accidental broker-token reuse 방지 설정
- runtime request/attempt journal domain contract
- outbox lease, exact-expiry fencing, retry, HMAC signing과 receiver verification
- local durable state corruption detection, legacy migration, chunk hydration and compaction
- virtual-key v2·legacy parser
- migration load·ordering·checksum contract
- local IPC roundtrip
- update staging과 storage contract
- 40 KiB audit, 비정규 파일 거절, strict chunked manifest, aggregate 검증과 deterministic source archive

## 운영 진입 조건

1. clean DB와 이전 production schema에서 모든 migration을 실제 수행한다.
2. PostgreSQL integration test를 전용 DB에서 통과하고, multi-replica Control·outbox contention을 soak test한다.
3. Valkey standalone·Sentinel 또는 cluster 장애에서 lease fail-closed를 검증한다.
4. 실제 provider별 non-stream·stream·tool·reasoning·429·중간 EOF fixture를 통과한다.
5. provider reported usage와 invoice reconciliation 오차 기준을 확정한다.
6. money-platform의 quote–hold–capture–release와 signed outbox event를 E2E 연결한다.
7. Windows Wails lifecycle, named-pipe ACL, installer overwrite, updater rollback을 실기기에서 통과한다.
8. Remote MCP를 OIDC/OAuth 또는 workload identity와 key rotation으로 확장한다.
9. provider credential을 OS keychain 또는 KMS로 이동한다.
10. 24시간 이상 soak test와 graceful shutdown 중 stream·lease·worker recovery를 검증한다.
