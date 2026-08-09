# 15. Build and Release

## Release Contract

A successful compile does not complete a release. The same revision must produce these artifacts and verification evidence:

- Server binaries: `gatewayd`, `controld`, `expert-brokerd`, `outboxd`
- Operations binaries: `dbmigrate`, `projectctl`, `keyctl`, `controlctl`, `mcptokenctl`, `outboxctl`, `webhook-sink`
- Local binaries: Wails app, `mcp-bridge`, `headless`
- Container images and immutable digests
- Database migration status
- Protobuf and sqlc contract generation
- SBOM, provenance, and signatures
- Signed desktop update manifest
- Versioned configuration and route examples

Generate `go.sum` and the root Bun workspace `bun.lock` with the reference toolchain while network access is available, and commit them with the release. CI and release jobs use only `GOFLAGS=-mod=readonly` and root-level `bun install --frozen-lockfile`; they do not allow implicit module edits, per-workspace lockfiles, or dependency drift. Do not create a formal release when a lockfile is missing or generated code is dirty.

Before expensive release jobs, `releasepack readiness --root . --version <version>` fails closed on the version, public Go module, `go.sum`, `bun.lock`, Apache-2.0 `LICENSE`, `NOTICE`, removal of pending-license markers, and source-manifest parity.

Each build job passes the SHA-256 list for generated contracts, source archives, server binaries, web assets, and Windows desktop bundles to `actions/attest@v4`, binding SLSA build provenance to the same job identity and commit. Each job also creates SPDX JSON with the Anchore SBOM Action `v0.24.0` and Apache-2.0 Syft `v1.50.0`, then issues a separate SBOM attestation for the same checksum subject. Disable automatic artifact and release uploads from the action; include the SBOM explicitly in the existing release-candidate artifact.

For a public repository, verify provenance and SBOM attestations with `gh attestation verify <artifact> --repo 0disoft/relaydock`. Private repositories require GitHub Enterprise Cloud for artifact attestations, so confirm repository visibility and plan before a formal public release.

Provenance and SBOM attestations do not replace an independent release-signing key. Artifact signatures and desktop updater-manifest signatures remain separate gates.

`releasepack sign-checksums` signs checksum files with a domain-separated payload using a base64 or base64url Ed25519 seed or private key from `RELAYDOCK_RELEASE_SIGNING_PRIVATE_KEY`. Never record the private-key value in CLI arguments or artifacts. `releasepack verify-checksums` uses a separate `RELAYDOCK_RELEASE_SIGNING_PUBLIC_KEY` to verify the schema, algorithm, key ID, subject digest, and signature in the strict JSON envelope. Do not reuse the artifact key for updater manifests, Control snapshots, or MCP tokens.

After every build matrix completes, the release workflow exposes the private-key secret only in the dedicated `sign-release-checksums` job. Excluding diagnostic artifacts created automatically by Buildx, it downloads release candidates, verifies that exactly 14 expected checksum files exist, signs each one, and immediately verifies it with the public key stored as a repository variable. Signature envelopes do not modify original artifacts and are retained in a separate `release-signatures` artifact. Pin the public-key trust root in release documentation or another organization channel, never in the same artifact as the signature.

`releasepack sign-update-manifest` normalizes an unsigned strict JSON manifest, then signs the same canonical payload used by the verifier -- `version`, artifact URL, SHA-256, and size -- with `RELAYDOCK_UPDATER_SIGNING_PRIVATE_KEY`. `releasepack verify-update-manifest` verifies the payload with a separate public-key environment variable. Signed output never overwrites an existing file, and the parser rejects unknown fields and trailing JSON. Do not create the manifest before the real artifact URL and update channel are finalized.

The container release-candidate job uses a pinned Docker Buildx action to create Linux/amd64 OCI archives for Gateway, Control, Expert, Outbox, Ops, and Webhook Sink. Each artifact includes the Buildx image digest, OCI tar SHA-256, SPDX SBOM, and archive-bound attestations. The job intentionally performs no registry login or push. After choosing a registry, import the same tested OCI content without rebuilding and issue a separate attestation for the registry-manifest digest to complete production promotion.

## Toolchain Pins

- Go: baseline in `go.mod`
- Wails CLI and module: the same exact alpha version
- Bun, Task, Buf, and sqlc: exact versions in bootstrap, Taskfile, and CI
- Generated bindings and clients: regenerate and compile with pinned generators in the release job; retain them as a separate checksum artifact

Do not update tools automatically. Upgrade pull requests must include generated diffs, protocol conformance, and Wails canary results.

## CI Gate

1. Check all Go formatting for drift.
2. Run `go test -count=1 ./...`.
3. Run the race detector on stateful and streaming packages.
4. Run `go vet ./...`.
5. Run `buf lint`, `buf generate`, and compile generated Go packages.
6. Run `sqlc generate` and compile the generated repository package.
7. Run frontend frozen install, typecheck, unit tests, and production build.
8. Apply migrations to a PostgreSQL 18 service.
9. Verify signed snapshots, runtime journal, transactional outbox, and fencing with the integration tag.
10. Verify migration up/down pairs and schema-snapshot parity.
11. Build server and operations binaries.
12. Compile the Wails frontend, bindings, desktop, and MCP bridge on a Windows runner.
13. Run container non-root smoke tests.
14. Produce artifact checksums and job-scoped provenance attestations.
15. Produce pinned Syft SPDX SBOMs and subject-bound SBOM attestations.
16. Produce unpublished OCI release candidates and image-digest evidence.
17. Promote registry digests and verify independent artifact and updater-manifest signatures.

PostgreSQL integration tests use only dedicated ephemeral databases and refuse to run without the reset opt-in environment variable.

## Server Release Gate

- Apply migrations successfully to both a clean database and the previous release schema.
- Round-trip down/up for the most recent rollback-capable migration.
- Soak PostgreSQL multi-worker claims and lease expiry.
- Verify real Valkey acquire, renew, release, and process-crash recovery.
- Test signed Control snapshot fetch, watch, LKG, and expiry.
- Test outbox receiver timeout, 429, permanent 4xx, and duplicate responses.
- Test provider non-stream, stream, tool, reasoning, 429, and EOF fixtures.
- Drain streams, workers, and outbox delivery during graceful shutdown.
- Verify a read-only container filesystem and writable state volumes.

Application replicas do not race to run migrations. Roll out only after a successful one-shot `dbmigrate` job.

## Desktop Release Gate

- Windows-first, per-user installation
- Exact Wails v3 version pin
- Signed updater manifest
- Stable and canary channels
- Wails app and MCP bridge packaged at the same version
- Stable absolute installed bridge path written to Codex configuration
- Retained rollback artifact for the previous version

### Wails Upgrade Gate

- App startup, tray, close-to-tray, and single-instance behavior
- Untrusted second-instance argument handling
- Binding generation
- Sleep/resume and WebView2 restart
- Updater signature failure and interrupted updates
- Reinstallation that preserves user data
- MCP bridge reconnection
- Windows Named Pipe ACLs and multi-user isolation

Do not expose Wails APIs outside `internal/desktopwails`. A failed canary must be recoverable by reverting only the desktop adapter.

## Secret and Policy Gate

- No `local/echo` route in production
- Bearer or virtual-key authentication on external binds
- Shared signing secret across Control replicas
- Only the public key, never the private key, provided to Gateway
- `GATEWAY_CONTROL_REQUIRED=true` and bounded snapshot TTL
- Scoped MCP token secret configured and Broker bearer reuse disabled
- TLS and a separate HMAC secret for outbox webhooks
- No provider credentials in images, manifests, or logs
- No development UUIDs, passwords, seeds, or HMAC secrets in production manifests
- Payload logging disabled by default

## Rollout Order

```text
backup and recovery checkpoint
  -> dbmigrate up
  -> controld canary / snapshot publish
  -> expert-brokerd workers
  -> gatewayd canary
  -> outboxd canary
  -> gatewayd and outboxd full rollout
  -> control-console
  -> desktop update channel publication
```

The Gateway canary verifies the new revision, swaps routes, and confirms that the request journal and outbox events are created correctly. Binary rollback is allowed only within the schema-compatibility window. Apply a forward fix when the previous version cannot interpret new data.
