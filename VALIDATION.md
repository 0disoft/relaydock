# Validation Report

검증 시점은 **2026-08-09**이며 대상 버전은 **`0.5.1-dev`**다. 이 문서는 현재 샌드박스에서 실제로 실행한 검사와, 외부 도구·서비스·운영체제·공급자 자격 증명이 없어 실행하지 못한 검사를 분리한다. CI workflow에 정의돼 있다는 사실을 실제 통과 결과로 간주하지 않는다.

## 검증 대상

최종 manifest 생성 직전의 수작업 소스와 생성된 manifest 조각을 합친 기준은 다음과 같다.

- 전체 regular file 446개
- Go 파일 288개, 약 31,173줄
- Go test 파일 49개, 명명된 `Test...` 함수 164개
- Markdown 문서 58개
- SQL 파일 13개
- Svelte 파일 13개
- TypeScript 파일 12개
- JSON 파일 12개
- YAML 파일 14개
- Proto 파일 4개
- 기준선: Go 1.26, Wails v3 `v3.0.0-alpha2.119`

`TREE.md`와 manifest는 `releasepack`이 source archive 직전에 다시 생성한다. root `MANIFEST.json`은 자기 자신과 `manifest/**`를 제외한 소스 record를 가리키며, 실제 record는 40 KiB보다 작은 정렬 chunk로 분리한다.

## 실제로 통과한 검사

### Dependency-free Go 범위

현재 실행 환경은 Go 1.23.2이며 네트워크가 차단되어 Go 1.26 toolchain과 외부 module을 내려받을 수 없다. 따라서 검사 중에만 `go.mod`의 language directive를 1.23으로 낮추고 `GOWORK=off GOTOOLCHAIN=local GOPROXY=off`를 사용했다. 검사 직후 원본을 복구했으며 artifact의 `go.mod`와 `go.work`는 모두 `go 1.26`이다.

`go list -e` 결과 중 외부 module 없이 완전하게 해석되는 **83개 package**를 자동 선별해 다음을 실행했다.

```text
go test -json -count=1   통과
test/subtest pass event  170개
fail event               0개
go test -race -count=1   통과
go vet                   통과
```

검증 범위에는 canonical protocol, provider HTTP adapter core, 실시간 stream state machine, retry 경계, deterministic routing, signed Control snapshot과 key ring, local Expert store v3, ContextPack chunk integrity, concurrent compaction, runtime domain, outbox, webhook, migration loader, local IPC, updater, file-size policy와 deterministic source packager가 포함된다.

다음 package는 외부 module이 없어 불완전하게 해석됐으므로 위 결과에 포함하지 않았다.

```text
Wails desktop root와 internal/desktopwails
MCP bridge·remote MCP 및 이를 포함한 command
pgx 기반 PostgreSQL package와 command
valkey-go 기반 Valkey package와 Gateway command
Connect·Wails·MCP·pgx 의존성을 전이적으로 포함하는 server command
```

이 제외 범위는 정식 Go 1.26 CI에서 전체 compile·test 대상으로 유지한다.

### 이번 변경에서 직접 검증한 경계

- 40 KiB를 넘는 수작업 source·문서·설정 파일이 없음
- root manifest와 3개 manifest chunk가 모두 40 KiB 이하
- file-size exception 0개, stale exception 0개
- symlink·socket·device 같은 비정규 파일을 archive에서 조용히 누락하지 않고 거절
- manifest와 size-policy JSON의 unknown field·trailing JSON 거절
- source ZIP entry 정렬, timestamp 정규화, archive 중 mode·size·SHA-256 재검증
- Windows에서도 일반 파일 `0644`, 셸 스크립트 `0755` mode를 보존하는 source manifest 정규화
- 동일 입력·timestamp의 source ZIP SHA-256 재현
- archive output의 repository 내부 배치 거절과 기존 output 원자 교체
- local Expert metadata와 최대 32 KiB content-addressed source chunk 분리
- store open 시 chunk 크기와 실제 SHA-256 전수검사
- 같은 크기로 변조된 source chunk 거절
- ContextPack write/read와 destructive compaction의 chunk lifecycle RW 동기화
- v1·v2 embedded ContextPack의 v3 migration과 원본 backup 보존
- ContextPack·consultation 원자 생성, immutable ID, idempotency conflict
- terminal graph·orphan result·orphan chunk compaction과 dry run
- Ed25519 `signingKeyId` active/retiring overlap, legacy no-ID 종료 스위치
- trusted signing-key JSON trailing value와 unsafe key ID 거절
- Remote MCP `argt2.<key-id>.<claims>.<hmac>` active/retiring key overlap
- normalized HMAC key ID 충돌, key count·secret size·claims size 상한
- token audience·subject·tenant·project·scope·ID·수명 검증

### 프론트엔드와 저장소 정적 검사

Node 22의 built-in TypeScript stripping으로 의존성 설치가 필요 없는 입력 검증 테스트를 실행했다.

```text
frontend consultation validation tests  3개 통과
fail                                  0개
```

다음 정적 검사도 실제로 통과했다.

```text
전체 Go 파일 gofmt drift 없음
Go package 디렉터리 99개 선언 일치
로컬 module import target 존재
Go에서 참조하는 환경 변수 116개 .env.example 문서화
JSON 12개 파싱
YAML 14개 파싱
Shell script 4개 sh -n
Proto 4개 syntax/package/go_package 선언
Migration 000001~000004 up/down 쌍과 순차성
VERSION·package.json·buildinfo·Go directive 일치
trailing whitespace 없음
0바이트 파일 없음
source tree symlink 없음
```

## Source archive 검증

`releasepack build`는 source tree를 다시 생성하고, manifest index·chunk를 작성한 뒤 즉시 자체 `verify`를 수행한다. 그 뒤 ZIP을 만들 때 각 source file의 mode·size·SHA-256을 다시 확인한다. 최종 artifact 생성 시 다음 검사를 반복한다.

```text
releasepack audit
releasepack verify
ZIP CRC test
ZIP extract 후 manifest verify
ZIP entry path traversal·duplicate 검사
ZIP 내부 모든 regular file 40 KiB 이하
ZIP 내부 MANIFEST index와 chunk hash 대조
```

최종 ZIP SHA-256은 artifact 바깥의 `.sha256` 파일에 기록한다. ZIP hash를 저장소 내부 문서에 넣지 않는 이유는 문서 수정이 다시 ZIP hash를 바꾸는 순환 참조를 만들기 때문이다.

## CI에 정의했지만 이 샌드박스에서 실행하지 못한 gate

다음은 workflow에 구성돼 있으나 이번 로컬 결과를 통과로 기록하지 않는다.

- Go 1.26 전체 non-desktop test·race·vet
- Buf lint·generate 후 generated Go compile
- sqlc generate 후 generated repository compile
- PostgreSQL 18 migration, runtime journal, outbox lease integration
- Bun/Svelte desktop·control-console build
- Windows runner의 pinned Wails v3 bindings·desktop·MCP bridge compile
- native release artifact와 checksum 생성

## 이 환경에서 수행하지 못한 운영 검증

다음 항목은 성공한 것으로 취급하지 않는다.

- Go 1.26 toolchain의 실제 `go test ./...`, `go test -race ./...`, `go vet ./...`
- Wails v3, MCP Go SDK, go-winio, pgx, valkey-go, Connect를 포함한 전체 module compile
- 네트워크 연결 환경의 `go mod tidy`, `go.sum` 생성과 dependency review
- Bun workspace install, lockfile 생성, Svelte/Vite production build
- `buf lint`, `buf breaking`, `buf generate`, `sqlc generate`
- 실제 PostgreSQL 18 migration·rollback·multi-replica contention·backup restore
- 실제 Valkey standalone·Sentinel·cluster Lua lease와 network partition
- Docker·Compose·Coolify image build와 non-root smoke test
- Windows Named Pipe ACL, Wails tray·single instance·autostart·installer·updater rollback
- 실제 OpenAI·Anthropic·Google·DeepSeek·OpenRouter stream/tool/reasoning conformance
- provider-reported usage와 invoice reconciliation
- Remote MCP OIDC/OAuth와 실제 ChatGPT·Codex connector conformance
- OS Credential Manager·Keychain·Secret Service 및 KMS adapter
- money-platform quote–hold–capture–release의 signed outbox end-to-end 정산
- 24시간 이상 soak test와 process kill 이후 stream·lease·worker recovery

## Lockfile 주의

현재 artifact에는 `go.sum`과 Bun lockfile이 없다. 외부 module을 내려받지 못하는 환경에서 검증되지 않은 checksum을 만들어 넣지 않았다. 정식 개발 환경에서는 다음을 먼저 실행하고 생성 결과를 리뷰해 커밋해야 한다.

```powershell
go mod tidy
bun install
```

## 개발 PC의 첫 검증 순서

```powershell
./scripts/rename-module.ps1 -NewModule "github.com/<owner>/<repo>"
./scripts/bootstrap.ps1
go mod tidy
bun install
./scripts/generate.ps1
./scripts/check.ps1
wails3 doctor
wails3 build
task package
```

관리형 경로는 전용 PostgreSQL·Valkey에서 migration, project bootstrap, key 발급, signed Control publish, Gateway journal, outbox sink 순서로 검증한다. 상세 절차는 `docs/15-build-and-release.md`, `docs/20-operations-runbook.md`, `docs/21-control-snapshot-distribution.md`, `docs/22-runtime-journal-and-outbox.md`, `docs/25-local-expert-store.md`, `docs/26-signing-and-token-key-rotation.md`, `docs/27-repository-size-and-source-release.md`를 따른다.
