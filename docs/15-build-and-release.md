# 15. Build and Release

## Release contract

릴리스는 compile 성공만으로 완료되지 않는다. 같은 revision에서 다음 artifact와 검증 근거가 생성돼야 한다.

- server binaries: `gatewayd`, `controld`, `expert-brokerd`, `outboxd`
- operations binaries: `dbmigrate`, `projectctl`, `keyctl`, `controlctl`, `mcptokenctl`, `outboxctl`, `webhook-sink`
- local binaries: Wails app, `mcp-bridge`, `headless`
- container image와 immutable digest
- database migration status
- protobuf/sqlc contract generation
- SBOM, provenance, signature
- signed desktop update manifest
- versioned config와 route examples

`go.sum`과 루트 Bun workspace의 `bun.lock`은 네트워크가 연결된 기준 toolchain에서 생성하고 release commit에 포함한다. CI와 release job은 `GOFLAGS=-mod=readonly`와 저장소 루트의 `bun install --frozen-lockfile`만 사용하며 암묵적인 module 수정, 하위 workspace별 lockfile, dependency drift를 허용하지 않는다. lockfile이 없거나 generated code가 dirty인 상태에서는 정식 release를 만들지 않는다.

`releasepack readiness --root . --version <version>`은 version, public Go module, `go.sum`, `bun.lock`, `LICENSE`, pending-license 제거와 source manifest 일치를 expensive release job 전에 fail-closed 검사한다.

generated contracts, source archive, server binaries, web assets, Windows desktop bundle은 각 build job이 만든 SHA-256 목록을 `actions/attest@v4`에 전달해 같은 job identity와 commit에 묶인 SLSA build provenance를 발급한다. 각 job은 Apache-2.0 Syft `v1.50.0`을 사용하는 Anchore SBOM Action `v0.24.0`으로 SPDX JSON을 만들고, 같은 checksum subject에 별도 SBOM attestation을 발급한다. action의 자동 artifact·release upload는 끄고 기존 release-candidate artifact에 SBOM을 명시적으로 포함한다.

공개 저장소에서는 `gh attestation verify <artifact> --repo 0disoft/relaydock`로 provenance와 SBOM attestation을 검증한다. 비공개 저장소는 GitHub Enterprise Cloud가 아니면 artifact attestation을 사용할 수 없으므로 정식 공개 release 전에 저장소 visibility와 plan을 확인한다.

provenance와 SBOM attestation은 독립 release signing key를 대체하지 않는다. release artifact 서명과 desktop updater manifest 서명은 각각 별도 gate로 통과해야 한다.

`releasepack sign-checksums`는 `RELAYDOCK_RELEASE_SIGNING_PRIVATE_KEY`의 base64/base64url Ed25519 seed 또는 private key로 checksum 파일을 domain-separated 서명한다. private key 값은 CLI argument나 artifact에 기록하지 않는다. `releasepack verify-checksums`는 별도 `RELAYDOCK_RELEASE_SIGNING_PUBLIC_KEY`로 strict JSON signature envelope의 schema, algorithm, key ID, subject digest와 signature를 모두 검증한다. release artifact key는 updater manifest key, Control snapshot key, MCP token key와 재사용하지 않는다.

release workflow는 모든 build matrix가 끝난 뒤 전용 `sign-release-checksums` job에서만 private key secret을 노출한다. Buildx가 자동 생성한 diagnostic artifact를 제외하고 release-candidate artifact를 내려받아 예상 checksum 파일 14개가 정확히 존재하는지 검사한 뒤 각각 서명하고 즉시 repository variable의 공개키로 검증한다. signature envelope는 원본 artifact를 변경하지 않고 별도 `release-signatures` artifact로 보존한다. 공개키 trust root는 signature와 같은 artifact에서 신뢰하지 말고 release 문서나 별도 조직 채널에 고정해야 한다.

`releasepack sign-update-manifest`는 unsigned strict JSON manifest를 정규화한 뒤 verifier와 동일한 `version`, artifact URL, SHA-256, size canonical payload를 `RELAYDOCK_UPDATER_SIGNING_PRIVATE_KEY`로 서명한다. `releasepack verify-update-manifest`는 별도 public-key 환경변수로 같은 payload를 검증한다. signed output은 기존 파일을 덮어쓰지 않으며, unknown field와 trailing JSON을 거절한다. 실제 artifact URL과 update channel이 확정되기 전에는 manifest를 만들지 않는다.

container release-candidate job은 pinned Docker Buildx action으로 Gateway, Control, Expert, Outbox, Ops, Webhook Sink의 Linux/amd64 OCI archive를 만든다. 각 artifact에는 Buildx image digest, OCI tar SHA-256, SPDX SBOM과 archive-bound attestations가 포함된다. 이 job은 registry login과 push를 명시적으로 하지 않는다. registry가 확정된 뒤 같은 tested OCI content를 rebuild 없이 import하고 registry manifest digest에 대한 별도 attestation을 발급해야 production promotion이 완료된다.

## Toolchain pins

- Go: `go.mod` 기준선
- Wails CLI와 module: 동일 exact alpha version
- Bun, Task, Buf, sqlc: bootstrap·Taskfile·CI의 exact version
- generated bindings와 clients: 고정 generator로 release job에서 재생성·컴파일하고 별도 checksum artifact로 보존

도구를 자동 최신화하지 않는다. 갱신 PR에는 generated diff, protocol conformance, Wails canary 결과가 포함돼야 한다.

## CI gate

1. 전체 Go formatting drift 확인
2. `go test -count=1 ./...`
3. stateful·streaming package의 race detector
4. `go vet ./...`
5. `buf lint`, `buf generate`, generated Go package compile
6. `sqlc generate`, generated repository package compile
7. frontend frozen install, typecheck, unit test, production build
8. PostgreSQL 18 service에 migration up
9. integration tag로 signed snapshot, runtime journal, transactional outbox, fencing 검증
10. migration up/down pair와 schema snapshot 일치 검사
11. server·ops binary build
12. Windows runner에서 Wails frontend·bindings·desktop·MCP bridge compile
13. container non-root smoke test
14. artifact checksum과 job-scoped provenance attestation
15. pinned Syft SPDX SBOM과 subject-bound SBOM attestation
16. unpublished OCI release-candidate와 image digest evidence
17. registry digest promotion과 독립 artifact·updater-manifest signature

PostgreSQL integration test는 전용 일회성 DB만 사용한다. reset opt-in 환경 변수가 없으면 실행을 거절한다.

## Server release gate

- clean DB와 이전 release schema 모두 migration up 통과
- rollback 가능한 마지막 migration의 down/up 왕복
- PostgreSQL multi-worker claim과 lease expiry soak
- 실제 Valkey acquire·renew·release와 process crash recovery
- signed Control snapshot fetch·watch·LKG·expiry test
- outbox receiver timeout·429·permanent 4xx·duplicate response test
- provider non-stream·stream·tool·reasoning·429·EOF fixture
- graceful shutdown 중 stream, worker, outbox drain
- container read-only filesystem과 writable state volume 확인

Migration을 application replica가 경쟁 실행하지 않는다. `dbmigrate` one-shot job 성공 뒤 rollout한다.

## Desktop release gate

- Windows first, per-user install
- Wails v3 exact version pin
- signed updater manifest
- stable/canary channel
- Wails app와 MCP bridge 같은 version으로 package
- Codex config에는 설치 후 고정된 absolute bridge path 기록
- previous version rollback artifact 보존

### Wails upgrade gate

- app start, tray, close-to-tray, single instance
- second-instance untrusted argument handling
- binding generation
- sleep/resume와 WebView2 restart
- updater signature failure와 interrupted update
- reinstall preserving user data
- MCP bridge reconnect
- Windows Named Pipe ACL and multi-user isolation

Wails API는 `internal/desktopwails` 밖으로 노출하지 않는다. canary 실패 시 desktop adapter만 되돌릴 수 있어야 한다.

## Secret and policy gate

- production route에 `local/echo`가 없음
- 외부 bind에 bearer 또는 virtual-key auth가 있음
- Control replica가 같은 signing secret을 사용함
- Gateway에는 private key가 아니라 public key만 있음
- `GATEWAY_CONTROL_REQUIRED=true`와 bounded snapshot TTL 적용
- scoped MCP token secret이 있고 Broker bearer reuse가 false임
- outbox webhook이 TLS와 별도 HMAC secret을 사용함
- provider credential이 image, manifest, log에 없음
- development UUID·password·seed·HMAC secret이 production manifest에 없음
- payload logging 기본 비활성화

## Rollout order

```text
backup and recovery checkpoint
  → dbmigrate up
  → controld canary / snapshot publish
  → expert-brokerd workers
  → gatewayd canary
  → outboxd canary
  → gatewayd and outboxd full rollout
  → control-console
  → desktop update channel publication
```

Gateway canary는 new revision을 검증해 route swap한 뒤 request journal과 outbox event가 정상 생성되는지 확인한다. Binary rollback은 schema compatibility window 안에서만 수행한다. 구버전이 새 데이터를 해석하지 못하면 forward fix를 적용한다.
