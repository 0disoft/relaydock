# 03. Repository Map

## Root

Wails v3 desktop entrypoint, workspace metadata, version, build contract를 둔다. protocol·routing·billing 로직을 두지 않는다.

## `cmd/`

독립 실행 진입점만 둔다.

| 디렉터리 | 책임 |
|---|---|
| `gatewayd/` | API data plane 조립과 process lifecycle |
| `controld/` | signed snapshot control plane |
| `expert-brokerd/` | consultation API, worker, Remote MCP |
| `mcp-bridge/` | STDIO MCP ↔ local IPC bridge |
| `headless/` | GUI 없는 local runtime |
| `dbmigrate/` | embedded migration runner CLI |
| `projectctl/` | organization/project bootstrap CLI |
| `keyctl/` | PostgreSQL virtual key 관리 CLI |

명령 package에 도메인 규칙을 복제하지 않는다. validation과 persistence 계약은 `internal/` package에 둔다.

## `internal/protocol/`

wire protocol과 canonical representation. provider credential, DB, UI를 알지 못한다.

## `internal/provider/`

provider 호출 adapter와 provider error taxonomy. routing 정책과 customer billing을 소유하지 않는다.

## `internal/composition/`

실행물별 dependency composition. 환경 변수와 adapter를 domain interface로 조립한다.

## `internal/routing/`

capability filter, health, cost, affinity, lease를 사용해 후보를 선택한다. HTTP payload를 직접 파싱하지 않는다. `distributedlease/`는 Valkey Lua 계약을 소유한다.

## `internal/runtime/`

attempt lifecycle, retry boundary, lease renewal, semantic commit을 소유한다. ingress 응답 형식은 알지 못한다.

## `internal/expert/`

consultation, ContextPack, redaction, result contract, local/PostgreSQL repository, worker, expert route를 소유한다.

## `internal/desktopwails/`

Wails와 domain 사이 adapter다. Wails import는 여기서 끝나야 한다.

## `internal/persistence/`

PostgreSQL, Valkey, atomic file, object store, migration adapter. 도메인 package가 pgx·valkey type을 노출하지 않게 한다.

## `internal/transport/`

HTTP ingress, SSE encoder, authentication middleware. domain state를 직접 소유하지 않는다.

## `config/`

검토 가능한 example route와 배포 설정. 실제 secret이나 provider credential을 넣지 않는다.

## `proto/`

서비스 간 공개 계약. 데이터베이스 모델을 그대로 Proto로 노출하지 않는다.

## `db/`

embedded migration, schema reference, sqlc query. 이미 적용된 migration을 수정하지 않는다.

## `frontend/`

Wails desktop UI. 생성 binding을 adapter 뒤에서 호출한다.

## `web/control-console/`

관리형 Control Plane UI. Desktop UI와 배포 경계를 공유하지 않는다.

## `tests/`

protocol conformance, golden stream, fault injection, expert worker, storage, accounting, load test를 둔다. 실제 provider credential이 필요한 시험은 별도 opt-in suite로 분리한다.
