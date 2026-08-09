# Repository Tree

```text
relaydock
├── .agents/
│   ├── checklists/
│   │   ├── cli-tool.md
│   │   ├── desktop-app.md
│   │   ├── monorepo.md
│   │   ├── ops-change.md
│   │   └── security.md
│   ├── skills/
│   │   ├── bugfix/
│   │   │   └── SKILL.md
│   │   ├── cli-tool/
│   │   │   └── SKILL.md
│   │   ├── desktop-app/
│   │   │   └── SKILL.md
│   │   ├── feature/
│   │   │   └── SKILL.md
│   │   └── monorepo/
│   │       └── SKILL.md
│   ├── context-map.md
│   └── README.md
├── .github/
│   ├── workflows/
│   │   ├── ci.yml
│   │   ├── desktop-canary.yml
│   │   └── release.yml
│   └── PULL_REQUEST_TEMPLATE.md
├── .ssealed/
│   └── manifest.json
├── build/
│   ├── darwin/
│   │   └── Taskfile.yml
│   ├── linux/
│   │   └── Taskfile.yml
│   ├── windows/
│   │   └── Taskfile.yml
│   ├── config.yml
│   ├── README.md
│   └── Taskfile.yml
├── cmd/
│   ├── controlctl/
│   │   └── main.go
│   ├── controld/
│   │   └── main.go
│   ├── dbmigrate/
│   │   └── main.go
│   ├── expert-brokerd/
│   │   └── main.go
│   ├── expertstorectl/
│   │   └── main.go
│   ├── gatewayd/
│   │   ├── control_snapshot.go
│   │   ├── control_snapshot_keys.go
│   │   └── main.go
│   ├── headless/
│   │   └── main.go
│   ├── keyctl/
│   │   └── main.go
│   ├── mcp-bridge/
│   │   └── main.go
│   ├── mcptokenctl/
│   │   └── main.go
│   ├── outboxctl/
│   │   └── main.go
│   ├── outboxd/
│   │   └── main.go
│   ├── projectctl/
│   │   └── main.go
│   ├── releasepack/
│   │   └── main.go
│   └── webhook-sink/
│       └── main.go
├── config/
│   ├── file-size-exceptions.json
│   ├── gateway-routes.example.json
│   └── runtime-snapshot.example.json
├── db/
│   ├── migrations/
│   │   ├── 000001_initial.down.sql
│   │   ├── 000001_initial.up.sql
│   │   ├── 000002_expert_durability.down.sql
│   │   ├── 000002_expert_durability.up.sql
│   │   ├── 000003_runtime_journal.down.sql
│   │   ├── 000003_runtime_journal.up.sql
│   │   ├── 000004_outbox_delivery.down.sql
│   │   └── 000004_outbox_delivery.up.sql
│   ├── queries/
│   │   ├── control.sql
│   │   ├── expert.sql
│   │   ├── outbox.sql
│   │   └── runtime.sql
│   ├── migrate.go
│   ├── README.md
│   └── schema.sql
├── deploy/
│   ├── caddy/
│   │   └── Caddyfile
│   ├── coolify/
│   │   └── README.md
│   ├── docker/
│   │   ├── Dockerfile.control
│   │   ├── Dockerfile.expert
│   │   ├── Dockerfile.gateway
│   │   ├── Dockerfile.ops
│   │   ├── Dockerfile.outbox
│   │   └── Dockerfile.webhook-sink
│   ├── docker-compose.dev.yml
│   └── README.md
├── docs/
│   ├── adr/
│   │   ├── 0001-go-first.md
│   │   ├── 0002-wails-v3.md
│   │   ├── 0003-local-ipc.md
│   │   ├── 0004-no-web-automation.md
│   │   ├── 0005-postgres-outbox.md
│   │   ├── 0006-protocol-loss-modes.md
│   │   ├── 0007-money-boundary.md
│   │   ├── 0008-license-boundary.md
│   │   ├── 0009-mcp-sdk-version.md
│   │   └── 0010-wails-version-pin.md
│   ├── architecture/
│   │   └── 00-system-boundary.md
│   ├── cli/
│   │   ├── command-contract.md
│   │   └── README.md
│   ├── desktop/
│   │   ├── installers.md
│   │   └── README.md
│   ├── engineering/
│   │   └── 00-project-invariants.md
│   ├── monorepo/
│   │   ├── README.md
│   │   └── workspace-boundaries.md
│   ├── ops/
│   │   └── 00-operational-contract.md
│   ├── product/
│   │   ├── 00-product-brief.md
│   │   └── 02-spec.md
│   ├── 00-product-identity.md
│   ├── 01-scope-and-non-goals.md
│   ├── 02-system-context.md
│   ├── 03-repository-map.md
│   ├── 04-request-lifecycle.md
│   ├── 05-expert-escalation.md
│   ├── 06-mcp-contract.md
│   ├── 07-context-pack.md
│   ├── 08-protocol-compiler.md
│   ├── 09-routing.md
│   ├── 10-accounting.md
│   ├── 11-security-threat-model.md
│   ├── 12-data-model.md
│   ├── 13-observability.md
│   ├── 14-testing-strategy.md
│   ├── 15-build-and-release.md
│   ├── 16-implementation-order.md
│   ├── 17-open-questions.md
│   ├── 18-api-conventions.md
│   ├── 19-ui-information-architecture.md
│   ├── 20-operations-runbook.md
│   ├── 21-control-snapshot-distribution.md
│   ├── 22-runtime-journal-and-outbox.md
│   ├── 23-remote-mcp-security.md
│   ├── 24-development-stack.md
│   ├── 25-local-expert-store.md
│   ├── 26-signing-and-token-key-rotation.md
│   ├── 27-repository-size-and-source-release.md
│   └── README.md
├── frontend/
│   ├── bindings/
│   │   └── README.md
│   ├── src/
│   │   ├── components/
│   │   │   ├── AppShell.svelte
│   │   │   ├── ConsultationCard.svelte
│   │   │   ├── ContextPreview.svelte
│   │   │   ├── ProviderCard.svelte
│   │   │   ├── Sidebar.svelte
│   │   │   └── StatusBadge.svelte
│   │   ├── lib/
│   │   │   ├── consultation-validation.test.ts
│   │   │   ├── consultation-validation.ts
│   │   │   ├── runtime-adapter.ts
│   │   │   ├── types.ts
│   │   │   └── wails-services.ts
│   │   ├── pages/
│   │   │   ├── Architect.svelte
│   │   │   ├── MCP.svelte
│   │   │   ├── Overview.svelte
│   │   │   ├── Providers.svelte
│   │   │   └── Settings.svelte
│   │   ├── App.svelte
│   │   ├── main.ts
│   │   ├── styles.css
│   │   └── vite-env.d.ts
│   ├── index.html
│   ├── package.json
│   ├── README.md
│   ├── tsconfig.json
│   ├── uno.config.ts
│   └── vite.config.ts
├── gen/
│   └── go/
│       ├── control/
│       │   └── v1/
│       │       ├── controlv1connect/
│       │       │   └── control.connect.go
│       │       └── control.pb.go
│       ├── expert/
│       │   └── v1/
│       │       ├── expertv1connect/
│       │       │   └── expert.connect.go
│       │       └── expert.pb.go
│       ├── money/
│       │   └── v1/
│       │       ├── moneyv1connect/
│       │       │   └── money.connect.go
│       │       └── money.pb.go
│       ├── runtime/
│       │   └── v1/
│       │       ├── runtimev1connect/
│       │       │   └── runtime.connect.go
│       │       └── runtime.pb.go
│       └── README.md
├── internal/
│   ├── accounting/
│   │   ├── memory.go
│   │   ├── money_client.go
│   │   ├── service.go
│   │   └── types.go
│   ├── appdirs/
│   │   ├── paths.go
│   │   └── paths_test.go
│   ├── auth/
│   │   ├── authorization/
│   │   │   ├── scopes.go
│   │   │   └── set.go
│   │   ├── mcpconfig/
│   │   │   ├── static_token.go
│   │   │   └── static_token_test.go
│   │   ├── mcpscope/
│   │   │   ├── scope.go
│   │   │   └── scope_test.go
│   │   ├── scopedtoken/
│   │   │   ├── claims.go
│   │   │   ├── context.go
│   │   │   ├── keyring.go
│   │   │   ├── secrets.go
│   │   │   ├── service.go
│   │   │   ├── token_test.go
│   │   │   ├── types.go
│   │   │   └── validation.go
│   │   └── virtualkey/
│   │       ├── context.go
│   │       ├── memory.go
│   │       ├── parser.go
│   │       ├── parser_test.go
│   │       ├── postgres.go
│   │       └── service.go
│   ├── autostart/
│   │   ├── memory.go
│   │   └── service.go
│   ├── buildinfo/
│   │   └── info.go
│   ├── composition/
│   │   └── gateway/
│   │       ├── builder.go
│   │       ├── builder_test.go
│   │       ├── routes_config.go
│   │       ├── routes_health.go
│   │       ├── routes_snapshot.go
│   │       ├── routes_source.go
│   │       ├── routes_test.go
│   │       ├── routes_types.go
│   │       └── snapshot_test.go
│   ├── config/
│   │   └── config.go
│   ├── control/
│   │   ├── modelregistry/
│   │   │   ├── memory.go
│   │   │   └── model.go
│   │   ├── policy/
│   │   │   ├── types.go
│   │   │   └── validate.go
│   │   └── snapshot/
│   │       ├── keyfile.go
│   │       ├── keyparse.go
│   │       ├── keyparse_test.go
│   │       ├── keyring.go
│   │       ├── keyring_test.go
│   │       ├── local_store.go
│   │       ├── manager.go
│   │       ├── manager_test.go
│   │       ├── memory_store.go
│   │       ├── public_key.go
│   │       ├── remote_client.go
│   │       ├── remote_client_test.go
│   │       ├── signer.go
│   │       ├── store.go
│   │       ├── types.go
│   │       └── validate.go
│   ├── core/
│   │   └── errors.go
│   ├── credentials/
│   │   ├── memory.go
│   │   └── store.go
│   ├── desktopwails/
│   │   ├── app.go
│   │   ├── consultation_service.go
│   │   ├── container.go
│   │   ├── runtime_service.go
│   │   ├── settings_service.go
│   │   ├── tray.go
│   │   └── wails_autostart.go
│   ├── expert/
│   │   ├── app/
│   │   │   └── app.go
│   │   ├── consultation/
│   │   │   ├── idempotency.go
│   │   │   ├── memory_repository.go
│   │   │   ├── repository.go
│   │   │   ├── service.go
│   │   │   ├── state_machine.go
│   │   │   ├── types.go
│   │   │   ├── work.go
│   │   │   └── work_logic.go
│   │   ├── contextpack/
│   │   │   ├── compiler.go
│   │   │   ├── manifest.go
│   │   │   ├── memory_store.go
│   │   │   ├── selector.go
│   │   │   ├── store.go
│   │   │   └── types.go
│   │   ├── localstore/
│   │   │   ├── adapters.go
│   │   │   ├── atomic_create.go
│   │   │   ├── chunks.go
│   │   │   ├── clone.go
│   │   │   ├── compaction.go
│   │   │   ├── consultations.go
│   │   │   ├── contextpacks.go
│   │   │   ├── expiry.go
│   │   │   ├── open.go
│   │   │   ├── results.go
│   │   │   ├── state.go
│   │   │   ├── store_test.go
│   │   │   ├── transaction.go
│   │   │   ├── types.go
│   │   │   └── work.go
│   │   ├── policy/
│   │   │   └── delegation.go
│   │   ├── postgresstore/
│   │   │   ├── consultation.go
│   │   │   ├── consultation_create.go
│   │   │   ├── contextpack.go
│   │   │   ├── result.go
│   │   │   ├── scan.go
│   │   │   ├── store.go
│   │   │   └── work.go
│   │   ├── redaction/
│   │   │   ├── rules.go
│   │   │   └── scanner.go
│   │   ├── resultcontract/
│   │   │   ├── clone.go
│   │   │   ├── memory_store.go
│   │   │   ├── record.go
│   │   │   ├── types.go
│   │   │   └── validator.go
│   │   ├── routes/
│   │   │   ├── openaiapi/
│   │   │   │   └── client.go
│   │   │   └── webhandoff/
│   │   │       └── service.go
│   │   └── worker/
│   │       ├── openai_executor.go
│   │       └── worker.go
│   ├── generateddeps/
│   │   └── deps.go
│   ├── identifier/
│   │   ├── uuid.go
│   │   └── uuid_test.go
│   ├── idgen/
│   │   └── id.go
│   ├── localipc/
│   │   ├── client.go
│   │   ├── endpoint_unix.go
│   │   ├── endpoint_windows.go
│   │   ├── protocol.go
│   │   ├── server.go
│   │   ├── transport_unix.go
│   │   └── transport_windows.go
│   ├── localruntime/
│   │   └── runtime.go
│   ├── mcpbridge/
│   │   ├── backend.go
│   │   ├── server.go
│   │   └── tools.go
│   ├── mcpcontract/
│   │   └── types.go
│   ├── mcpremote/
│   │   ├── backend.go
│   │   ├── backend_test.go
│   │   ├── server.go
│   │   └── types.go
│   ├── observability/
│   │   ├── logger.go
│   │   ├── metrics.go
│   │   └── tracing.go
│   ├── outbox/
│   │   ├── webhooksink/
│   │   │   ├── handler.go
│   │   │   └── handler_test.go
│   │   ├── admin.go
│   │   ├── delivery.go
│   │   ├── delivery_test.go
│   │   ├── memory.go
│   │   ├── memory_delivery.go
│   │   ├── types.go
│   │   ├── webhook.go
│   │   └── webhook_test.go
│   ├── persistence/
│   │   ├── atomicfile/
│   │   │   ├── file.go
│   │   │   ├── permissions_unix.go
│   │   │   ├── permissions_windows.go
│   │   │   ├── replace_unix.go
│   │   │   └── replace_windows.go
│   │   ├── migrate/
│   │   │   ├── migration.go
│   │   │   ├── migration_test.go
│   │   │   └── runner.go
│   │   ├── objectstore/
│   │   │   ├── memory.go
│   │   │   └── store.go
│   │   ├── postgres/
│   │   │   ├── sqlcgen/
│   │   │   │   ├── control.sql.go
│   │   │   │   ├── db.go
│   │   │   │   ├── expert.sql.go
│   │   │   │   ├── models.go
│   │   │   │   ├── outbox.sql.go
│   │   │   │   ├── querier.go
│   │   │   │   └── runtime.sql.go
│   │   │   ├── db.go
│   │   │   ├── outbox_repository.go
│   │   │   ├── runtime_journal.go
│   │   │   └── snapshot_store.go
│   │   └── valkey/
│   │       ├── client.go
│   │       └── lease_manager.go
│   ├── protocol/
│   │   ├── anthropic/
│   │   │   └── messages/
│   │   │       └── adapter.go
│   │   ├── canonical/
│   │   │   ├── capabilities.go
│   │   │   └── types.go
│   │   ├── compiler/
│   │   │   ├── compiler.go
│   │   │   ├── loss.go
│   │   │   └── registry.go
│   │   ├── defaults/
│   │   │   └── registry.go
│   │   ├── gemini/
│   │   │   └── generate/
│   │   │       └── adapter.go
│   │   ├── openai/
│   │   │   ├── chat/
│   │   │   │   └── adapter.go
│   │   │   └── responses/
│   │   │       └── adapter.go
│   │   └── stream/
│   │       ├── event.go
│   │       └── state_machine.go
│   ├── provider/
│   │   ├── anthropic/
│   │   │   └── adapter.go
│   │   ├── deepseek/
│   │   │   └── adapter.go
│   │   ├── google/
│   │   │   └── adapter.go
│   │   ├── httpadapter/
│   │   │   ├── adapter.go
│   │   │   ├── decode.go
│   │   │   └── response_stream.go
│   │   ├── mock/
│   │   │   └── adapter.go
│   │   ├── openai/
│   │   │   └── adapter.go
│   │   ├── openaicompatible/
│   │   │   └── adapter.go
│   │   ├── openrouter/
│   │   │   └── adapter.go
│   │   ├── adapter.go
│   │   ├── error.go
│   │   └── registry.go
│   ├── releasepack/
│   │   ├── archive.go
│   │   ├── integrity.go
│   │   ├── json.go
│   │   ├── manifest.go
│   │   ├── policy.go
│   │   ├── readiness.go
│   │   ├── releasepack_test.go
│   │   ├── replace_unix.go
│   │   ├── replace_windows.go
│   │   ├── scan.go
│   │   ├── tree.go
│   │   ├── types.go
│   │   └── verify.go
│   ├── routing/
│   │   ├── distributedlease/
│   │   │   ├── manager.go
│   │   │   ├── manager_test.go
│   │   │   └── scripts.go
│   │   ├── health.go
│   │   ├── lease.go
│   │   ├── memory.go
│   │   ├── outcome.go
│   │   ├── router.go
│   │   ├── router_test.go
│   │   ├── scorer.go
│   │   └── types.go
│   ├── runtime/
│   │   ├── attempt.go
│   │   ├── journal.go
│   │   ├── lease.go
│   │   ├── memory_journal.go
│   │   ├── memory_journal_test.go
│   │   ├── memory_source.go
│   │   ├── request.go
│   │   ├── retry.go
│   │   ├── run.go
│   │   └── types.go
│   ├── security/
│   │   ├── secrets/
│   │   │   ├── encrypted_memory.go
│   │   │   └── store.go
│   │   └── ssrf/
│   │       └── validator.go
│   ├── serverutil/
│   │   ├── exposure.go
│   │   └── server.go
│   ├── transport/
│   │   ├── apiutil/
│   │   │   ├── api.go
│   │   │   ├── auth.go
│   │   │   └── gateway_auth.go
│   │   ├── controlhttp/
│   │   │   ├── key_rotation_test.go
│   │   │   ├── server.go
│   │   │   └── server_test.go
│   │   ├── experthttp/
│   │   │   └── server.go
│   │   ├── health/
│   │   │   └── handler.go
│   │   └── httpgateway/
│   │       ├── handler.go
│   │       ├── middleware.go
│   │       ├── process.go
│   │       ├── response.go
│   │       ├── runtime_processor.go
│   │       ├── runtime_processor_test.go
│   │       ├── status_test.go
│   │       ├── streaming.go
│   │       └── types.go
│   └── updater/
│       ├── http_service.go
│       └── service.go
├── proto/
│   ├── control/
│   │   └── v1/
│   │       └── control.proto
│   ├── expert/
│   │   └── v1/
│   │       └── expert.proto
│   ├── money/
│   │   └── v1/
│   │       └── money.proto
│   └── runtime/
│       └── v1/
│           └── runtime.proto
├── scripts/
│   ├── bootstrap.ps1
│   ├── bootstrap.sh
│   ├── check.ps1
│   ├── check.sh
│   ├── clean.go
│   ├── dev.ps1
│   ├── format.ps1
│   ├── format.sh
│   ├── generate.ps1
│   ├── generate.sh
│   └── rename-module.ps1
├── tests/
│   ├── billing/
│   │   └── idempotency_test.go
│   ├── conformance/
│   │   ├── gateway_auth_test.go
│   │   ├── httpadapter_test.go
│   │   ├── protocol_test.go
│   │   └── provider_errors_test.go
│   ├── expert/
│   │   ├── consultation_test.go
│   │   ├── contextpack_test.go
│   │   ├── openaiapi_test.go
│   │   ├── worker_fence_test.go
│   │   └── worker_test.go
│   ├── fault_injection/
│   │   ├── fault_test.go
│   │   ├── http_adapter_stream_test.go
│   │   └── runtime_retry_test.go
│   ├── golden_streams/
│   │   └── stream_test.go
│   ├── hygiene/
│   │   └── public_identity_test.go
│   ├── integration/
│   │   ├── http_error_paths_test.go
│   │   ├── http_vertical_slice_test.go
│   │   ├── live_stream_test.go
│   │   ├── local_ipc_test.go
│   │   └── updater_test.go
│   ├── load/
│   │   └── gateway.js
│   ├── postgres/
│   │   └── runtime_outbox_integration_test.go
│   ├── storage/
│   │   ├── control_snapshot_store_test.go
│   │   ├── file_permissions_unix_test.go
│   │   ├── file_permissions_windows_test.go
│   │   ├── local_expert_store_test.go
│   │   └── stores_test.go
│   ├── testdata/
│   │   └── streams/
│   │       ├── anthropic_tool_delta.jsonl
│   │       └── openai_responses_text.jsonl
│   └── README.md
├── v3/
│   ├── commitlocks/
│   ├── commits/
│   ├── modulelocks/
│   ├── modules/
│   ├── plugins/
│   ├── policies/
│   ├── wasmruntime/
│   │   └── wazero-v1.12.0-amd64-windows/
│   └── wellknowntypes/
├── web/
│   ├── control-console/
│   │   ├── src/
│   │   │   ├── routes/
│   │   │   │   ├── +page.server.ts
│   │   │   │   └── +page.svelte
│   │   │   └── app.html
│   │   ├── package.json
│   │   ├── README.md
│   │   ├── svelte.config.js
│   │   ├── tsconfig.json
│   │   └── vite.config.ts
│   └── shared/
│       └── README.md
├── .dockerignore
├── .editorconfig
├── .env.example
├── .gitattributes
├── .gitignore
├── AGENTS.md
├── buf.gen.yaml
├── buf.yaml
├── bun.lock
├── CHANGELOG.md
├── CHECKLIST.md
├── config.example.yaml
├── CONTRIBUTING.md
├── go.mod
├── go.sum
├── go.work
├── IMPLEMENTATION_STATUS.md
├── LICENSE
├── main.go
├── NOTICE
├── package.json
├── README.md
├── SECURITY.md
├── sqlc.yaml
├── Taskfile.yml
├── VALIDATION.md
└── VERSION
```
