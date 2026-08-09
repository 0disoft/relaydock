# Validation Report

검증 시점은 **2026-08-09**이며 대상 버전은 **`0.5.2-dev`**다. 이 문서는 현재 샌드박스에서 실제로 실행한 검사와, 외부 도구·서비스·운영체제·공급자 자격 증명이 없어 실행하지 못한 검사를 분리한다. CI workflow에 정의돼 있다는 사실을 실제 통과 결과로 간주하지 않는다.

## 배포 준비 판정

**현재 판정은 repository release readiness 통과, external release blocked다.** source·license·dependency·local build gate는 통과했지만 hosted CI, signing key custody, registry와 실제 배포 환경 증거가 없으므로 public source release, binary/container publication, desktop update channel 개방은 아직 시작하지 않는다.

라이선스 경계는 2026-08-09에 저장소 전체 Apache-2.0으로 확정했다. 루트 `LICENSE`와 `NOTICE`를 추가하고 `LICENSE-PENDING.md`를 제거했으며, 모든 휴대용 bundle이 두 파일을 함께 포함하도록 회귀 검사를 통과했다.

`releasepack readiness --root . --version 0.5.2-dev`는 Go 1.26.4, `go.sum`, Bun 1.3.14 `bun.lock`, `LICENSE`, `NOTICE`, public module identity와 477개 source record의 현재 manifest를 확인하고 통과했다.

이번 준비도 보강에서 다음 항목은 로컬 검증을 통과했다.

- CI·release workflow의 Go read-only module resolution과 Bun frozen root-workspace install 계약
- generated contracts, source, server, web, Windows desktop 산출물별 GitHub build provenance 단계 존재
- 여섯 service Dockerfile의 `go.sum` 필수 입력, read-only module resolution, non-root runtime 계약
- release readiness·workflow·container hygiene 회귀 테스트
- source manifest 재생성·검증과 ssealed strict doctor
- Go 1.26.4 전체 package `go test -mod=readonly ./...`와 `go vet -mod=readonly ./...`
- Bun 1.3.14 lock-only resolution과 frozen install
- desktop·control-console `svelte-check` 0 errors·0 warnings, production build와 frontend test 3개

다음 항목은 workflow에 구성됐거나 문서 계약만 존재할 뿐 실제 성공 근거가 없다.

- hosted GitHub Actions 전체 실행과 artifact attestation 발급·검증
- hosted runner의 pinned Syft SPDX SBOM 생성·내용 검토·attestation 검증과 전용 Ed25519 key checksum signature 발급·외부 trust-root 검증
- hosted runner의 OCI release-candidate build·SBOM·attestation 검증과 registry push·immutable manifest digest promotion
- Windows native installer, code signing, updater manifest signing과 rollback
- PostgreSQL·Valkey·실제 provider·money-platform·24시간 soak 운영 gate

공개 저장소 visibility, artifact/code-signing key custody, container registry와 배포 대상은 코드가 대신 결정할 수 없는 release-owner 입력이다. 이 항목을 확정하고 외부 환경 gate를 실제 통과하기 전에는 “production-ready” 또는 “배포 완료”로 표시하지 않는다.

## 검증 대상

최종 manifest 생성 직전의 수작업 소스와 생성된 manifest 조각을 합친 기준은 다음과 같다.

- 전체 source record 477개
- Go 파일 295개
- Go test 파일 53개, 명명된 `Test...` 함수 182개
- Markdown 문서 82개
- SQL 파일 13개
- Svelte 파일 13개
- TypeScript 파일 11개
- JSON 파일 9개
- YAML 파일 14개
- Proto 파일 4개
- 기준선: Go 1.26, Wails v3 `v3.0.0-alpha2.119`

`TREE.md`와 manifest는 `releasepack`이 source archive 직전에 다시 생성한다. root `MANIFEST.json`은 자기 자신과 `manifest/**`를 제외한 소스 record를 가리키며, 실제 record는 40 KiB보다 작은 정렬 chunk로 분리한다.

## 실제로 통과한 검사

### 전체 Go dependency 범위

Go 1.26.4가 `go.mod`의 toolchain 기준을 선택했고, 격리된 module cache에서 `go mod tidy`로 `go.sum`을 생성했다. 이후 `GOPROXY=off`, `GOWORK=off`, `-mod=readonly`로 Wails, MCP, pgx, Valkey와 모든 command를 포함한 전체 package test를 통과했다. 같은 locked graph에서 전체 `go vet`도 통과했다.

Windows local runner에는 gcc·clang·zig가 없어 `CGO_ENABLED=1` race detector는 실행하지 못했다. GitHub Linux runner의 `go test -race` job을 실제 hosted release gate로 유지하며, workflow 존재만으로 통과했다고 기록하지 않는다.

### 이번 변경에서 직접 검증한 경계

- `github.com/0disoft/relaydock` module path로 protocol, runtime, routing, conformance, fault-injection package가 offline `-mod=readonly` 테스트를 통과
- RelayDock public identity hygiene와 source archive prefix 회귀 테스트 통과
- 40 KiB를 넘는 수작업 source·문서·설정 파일이 없음
- root manifest와 3개 manifest chunk가 모두 40 KiB 이하
- 분할할 수 없는 canonical `bun.lock` file-size exception 1개, stale exception 0개
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

Node 22의 built-in TypeScript stripping으로 입력 검증 테스트를 실행했다. 추가로 TypeScript 7 native compiler와 TypeScript 6 compatibility API를 사용하는 `svelte-check`를 두 workspace에 적용했다.

```text
frontend consultation validation tests  3개 통과
fail                                  0개
desktop svelte-check errors/warnings  0/0
control svelte-check errors/warnings  0/0
desktop Vite production build         통과
control adapter-node production build 통과
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

- Go 1.26 hosted Linux race detector
- Buf lint·generate 후 generated Go compile
- sqlc generate 후 generated repository compile
- PostgreSQL 18 migration, runtime journal, outbox lease integration
- Bun/Svelte desktop·control-console build
- Windows runner의 pinned Wails v3 bindings·desktop·MCP bridge compile
- native release artifact와 checksum 생성

## 이 환경에서 수행하지 못한 운영 검증

다음 항목은 성공한 것으로 취급하지 않는다.

- Windows local runner의 `go test -race ./...`와 hosted Linux race 결과
- Wails v3 native desktop 실행과 Windows Named Pipe transport의 실제 OS 통합
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

## Lockfile 계약

현재 artifact는 Go 1.26.4에서 생성한 `go.sum`과 Bun 1.3.14에서 생성한 root workspace `bun.lock`을 포함한다. CI와 release에서는 선언을 수정하지 않는 다음 frozen/read-only 동작만 허용한다.

```powershell
go test -mod=readonly ./...
bun install --frozen-lockfile
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
