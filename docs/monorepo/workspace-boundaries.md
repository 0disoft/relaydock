# Workspace Boundaries

- Status: Active index
- Repository type: multi-surface Go workspace
- Authority: [`../03-repository-map.md`](../03-repository-map.md)

RelayDock manages multiple executables and UIs in one repository without mixing responsibilities merely to share code.

- `cmd/`: executable entrypoints and composition
- `internal/`: protocol, provider, routing, runtime, expert, persistence, and transport boundaries
- `frontend/`: Wails desktop UI
- `web/control-console/`: managed Control Plane UI
- `proto/`, `db/`, `config/`: public contracts, migrations, and reviewable settings
- Generated directories: `frontend/bindings/`, `gen/go/`, `internal/persistence/postgres/sqlcgen/` -- do not edit directly

Follow [`../03-repository-map.md`](../03-repository-map.md) and [`../../AGENTS.md`](../../AGENTS.md) for exact package responsibilities and change rules.
