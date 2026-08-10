# Validation Report

Validation date: **2026-08-10**. Target version: **`0.5.9-dev`**. This report separates checks actually executed in the current sandbox or hosted CI from checks that could not run without external tools, services, operating systems, or provider credentials. A check defined in a workflow is not treated as a passing result.

## Release-Readiness Decision

**Repository release readiness passes; external release remains blocked.** Source, licensing, dependency, local build, and hosted CI gates pass, but public source or binary publication, container promotion, and a desktop update channel still require release-owner decisions for signing-key custody, registry, and deployment environments.

The repository-wide license boundary was finalized as Apache-2.0 on 2026-08-09. Root `LICENSE` and `NOTICE` are present, `LICENSE-PENDING.md` is removed, and portable-bundle regression checks require both files.

The full Go suite and `go vet` passed after wiring desktop and server provider-credential sources into Gateway composition. Both Svelte workspaces passed `svelte-check` with zero errors and zero warnings, both production builds completed, and all three desktop frontend tests passed. Source refresh produced 504 records in four chunks and verified the deterministic archive and manifest.

[GitHub Actions CI run 31301723465](https://github.com/0disoft/relaydock/actions/runs/31301723465) for commit `0905d93` passed all six jobs. It provides hosted-runner evidence for Linux race tests and vet, PostgreSQL 18 integration, Buf and SQLC generation plus generated-package compilation, Bun/Svelte checks, and Windows Wails desktop/MCP bridge compilation.

The following readiness additions passed locally:

- Windows Credential Manager adapter compilation plus opaque-target, copy-isolation, size, cancellation, missing-value, idempotent-delete, and secret-free-error tests
- macOS Keychain `SecItem` adapter source review and isolated build boundary without shell or process-argument secret transport; the dedicated hosted macOS job is the compilation gate
- Linux Secret Service session negotiation, opaque search attributes, locked-item prompt handling, create/read/delete behavior, duplicate rejection, transient-buffer zeroing, context propagation, and D-Bus error-body omission tests
- Desktop provider credential status, save, replace, delete, environment precedence, gateway-stop conflict, unavailable-store denial, secret non-disclosure, and gateway Authorization-header tests
- Server-secret reference parsing, GCP metadata-token and Secret Manager request shape, proxy bypass, redirect denial, response bounds, CRC32C verification, reference precedence, and fail-closed Gateway startup tests with deterministic fake HTTP transports
- OIDC discovery, exact issuer/audience/authorized-party/time/signature verification, explicit claim-to-role mapping, JWKS rotation, and static-bootstrap coexistence tests
- SSRF denial for private/mixed DNS results and validated-address dialing for issuer, redirect, and JWKS requests
- Deny-by-default Control role policy, SHA-256 static credential lookup, project model filtering, provider-account redaction, and safe access-decision audit tests
- Full Go package tests and vet with read-only, offline module resolution
- Read-only Go module resolution and frozen root-workspace Bun installation in CI and release workflows
- GitHub build-provenance steps for generated contracts, source, server, web, and Windows desktop artifacts
- Required `go.sum`, read-only modules, and non-root runtime contracts in six service Dockerfiles
- Release-readiness, workflow, and container-hygiene regression tests
- Source-manifest regeneration/verification and strict ssealed doctor
- Full `go test -mod=readonly ./...` and `go vet -mod=readonly ./...` on Go 1.26.4
- Bun 1.3.14 lock-only resolution and frozen installation
- Zero errors and warnings from desktop and Control Console `svelte-check`, successful production builds, and three frontend tests

The following are configured or documented but do not yet have successful execution evidence:

- Release-workflow artifact attestations and external verification
- Hosted Syft SPDX SBOM review/attestation and Ed25519 checksum signatures verified against an external trust root
- OCI release-candidate SBOM/attestation verification, registry push, and immutable manifest-digest promotion
- Windows native installers, code signing, updater-manifest signing, and rollback
- Live PostgreSQL, Valkey, provider, money-platform, and 24-hour soak operational gates
- Real external IdP conformance and the separate Control Console authorization-code/session flow
- Disposable-user physical Windows Credential Manager, macOS Keychain, and Linux Secret Service smoke tests
- Live Google Cloud attached-workload, secret-level IAM, metadata, Secret Manager, rotation, and denial smoke tests
- Workload-identity coverage for non-provider server secrets and non-Google secret-manager adapters

Do not label the product production-ready or deployed before those release-owner inputs and target-environment gates are complete.

## Validated Source Baseline

- 504 source records
- 320 Go files
- 64 Go test files with 230 named `Test...` functions
- 84 Markdown documents
- 13 SQL files
- 13 Svelte files
- 11 TypeScript files
- 9 JSON files
- 14 YAML files
- 4 Proto files
- Baseline: Go 1.26 and Wails v3 `v3.0.0-alpha2.119`

`releasepack` regenerates `TREE.md` and the manifest immediately before the source archive. Root `MANIFEST.json` references source records excluding itself and `manifest/**`; records are stored in sorted chunks below 40 KiB.

## Checks That Passed

### Full Go Dependency Scope

Go 1.26.4 selected the toolchain baseline from `go.mod`, and an isolated module cache produced `go.sum` through `go mod tidy`. Full-package tests then passed with `GOPROXY=off`, `GOWORK=off`, and `-mod=readonly`, including Wails, MCP, pgx, Valkey, and every command. Full `go vet` passed on the same locked graph.

The local Windows runner lacks gcc, clang, and zig, so it could not run the `CGO_ENABLED=1` race detector. The hosted Linux `go test -race` result above is the release-gate evidence.

### Directly Verified Boundaries

- Offline `-mod=readonly` tests for protocol, runtime, routing, conformance, and fault injection under `github.com/0disoft/relaydock`
- Public-identity hygiene and source-archive prefix regressions
- No handwritten source, documentation, or configuration over 40 KiB
- Root manifest and four manifest chunks below 40 KiB
- One indivisible canonical `bun.lock` exception and no stale exceptions
- Rejection of silently omitted symlinks, sockets, and devices
- Rejection of unknown fields and trailing JSON in manifests and size policy
- Sorted ZIP entries, normalized timestamps, and mode/size/SHA-256 revalidation during archive creation
- Windows source modes normalized to `0644` for regular files and `0755` for shell scripts
- Reproducible SHA-256 for identical source and timestamp
- Rejection of repository-internal archive output and atomic replacement of existing output
- Separate local Expert metadata and content-addressed source chunks of at most 32 KiB
- Full chunk-size and SHA-256 checks when opening a store, including same-size tamper rejection
- Chunk-lifecycle synchronization between reads/writes and destructive compaction
- v1/v2 embedded ContextPack migration to v3 with original backup retention
- Atomic ContextPack/consultation creation, immutable IDs, and idempotency conflicts
- Terminal graph, orphan result, and orphan chunk compaction with dry-run
- Ed25519 active/retiring key IDs, legacy no-ID retirement, strict trusted-key JSON, and safe key IDs
- Remote MCP argt2 active/retiring HMAC keys, normalized-ID collision checks, and size/count limits
- Token audience, subject, tenant, project, scope, ID, and lifetime validation
- Read-only `gcp-sm:` dispatch, fixed-origin metadata and Secret Manager calls, no-proxy metadata transport, redirect denial, bounded bodies, payload CRC32C, and secret-free error behavior

### Frontend and Static Checks

```text
frontend consultation validation tests  3 passed
fail                                  0
desktop svelte-check errors/warnings  0/0
control svelte-check errors/warnings  0/0
desktop Vite production build         passed
control adapter-node production build passed
```

The following static checks also passed:

```text
all Go files have no gofmt drift
99 Go package directories have consistent declarations
all local module import targets exist
116 environment variables referenced by Go are documented in .env.example
12 JSON files parse
14 YAML files parse
4 shell scripts pass sh -n
4 Proto files declare syntax, package, and go_package
migrations 000001-000004 have sequential up/down pairs
VERSION, package.json, buildinfo, and Go directive agree
no trailing whitespace
no zero-byte files
no source-tree symlinks
```

## Source Archive Verification

`releasepack build` regenerates the source tree and manifest index/chunks, runs its own verification, then rechecks mode, size, and SHA-256 while copying every source file into the ZIP. Final artifact verification repeats:

```text
releasepack audit
releasepack verify
ZIP CRC test
manifest verification after ZIP extraction
ZIP path-traversal and duplicate-entry checks
40 KiB ceiling for every regular file in the ZIP
MANIFEST index and chunk-hash comparison inside the ZIP
```

Record the final ZIP SHA-256 in an external `.sha256` file. Embedding it in repository documentation would create a circular hash dependency.

## Hosted CI Gates That Passed

- Go 1.26 Linux race detector and vet
- Buf 1.72.0 lint/generate plus Protobuf and ConnectRPC compilation
- SQLC 1.31.1 generation plus PostgreSQL repository compilation
- PostgreSQL 18 migrations, runtime journal, and outbox lease integration
- Bun frozen installation and desktop/Control Console Svelte checks
- Pinned Wails v3 bindings, desktop, and MCP bridge compilation on Windows

## Operational Validation Not Performed Here

Do not treat any of these as passing:

- Local Windows `go test -race ./...`
- Native Wails v3 execution and real Windows Named Pipe OS integration
- `buf breaking`
- Live PostgreSQL migration/rollback, multi-replica contention, backup, and restore
- Live Valkey standalone/Sentinel/cluster Lua leases and network partitions
- Docker, Compose, or Coolify image builds and non-root smoke tests
- Windows Named Pipe ACLs, tray, single instance, autostart, installers, and updater rollback
- Real OpenAI, Anthropic, Google, DeepSeek, or OpenRouter stream/tool/reasoning conformance
- Provider-reported usage and invoice reconciliation
- Remote MCP OIDC/OAuth and real ChatGPT/Codex connector conformance
- Physical OS credential-store smoke and live Google Cloud workload-identity/Secret Manager validation
- Workload-identity adapters for remaining server-secret consumers and non-Google deployment targets
- End-to-end money-platform quote/hold/capture/release through signed outbox events
- A 24-hour soak and stream/lease/worker recovery after process termination

## Lockfile Contract

Artifacts include `go.sum` generated with Go 1.26.4 and root `bun.lock` generated with Bun 1.3.14. CI and release use frozen/read-only operations only:

```powershell
go test -mod=readonly ./...
bun install --frozen-lockfile
```

## First Validation on a Development Workstation

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

For managed paths, validate migrations, project bootstrap, key issuance, signed Control publication, the Gateway journal, and the outbox sink against dedicated PostgreSQL and Valkey. Follow `docs/15-build-and-release.md`, `docs/20-operations-runbook.md`, `docs/21-control-snapshot-distribution.md`, `docs/22-runtime-journal-and-outbox.md`, `docs/25-local-expert-store.md`, `docs/26-signing-and-token-key-rotation.md`, and `docs/27-repository-size-and-source-release.md`.
