# Project Invariants Route

- Status: Active index
- Binding rules: [`../../AGENTS.md`](../../AGENTS.md)

`AGENTS.md` defines the invariants that implementation must preserve. In particular, never bypass these boundaries:

- No automatic provider retry after the first semantic event
- Reject losses in strict protocol conversion
- PostgreSQL alone owns authoritative billing state
- MCP messages are the only content allowed on STDIO stdout
- Do not log secrets or prompt/response bodies by default
- Do not spread Wails imports beyond `internal/desktopwails`

See [`../03-repository-map.md`](../03-repository-map.md) for structure, [`../../VALIDATION.md`](../../VALIDATION.md) for verification requirements, and [`../../CHECKLIST.md`](../../CHECKLIST.md) for task-specific checks. This page is not an independent source of rules.
