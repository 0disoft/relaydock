# Workspace Boundaries

- Status: Active index
- Repository type: multi-surface Go workspace
- Authority: [`../03-repository-map.md`](../03-repository-map.md)

RelayDock는 여러 실행물과 UI를 한 저장소에서 관리하지만, 공유 코드를 이유로 책임을 섞지 않는다.

- `cmd/`: 실행 진입점과 composition
- `internal/`: protocol, provider, routing, runtime, expert, persistence, transport의 책임 경계
- `frontend/`: Wails desktop UI
- `web/control-console/`: 관리형 Control Plane UI
- `proto/`, `db/`, `config/`: 공개 계약, migration, 검토 가능한 설정
- 생성 디렉터리: `frontend/bindings/`, `gen/go/`, `internal/persistence/postgres/sqlcgen/` — 직접 수정 금지

정확한 package 책임과 변경 규칙은 [`../03-repository-map.md`](../03-repository-map.md)와 [`../../AGENTS.md`](../../AGENTS.md)를 따른다.
