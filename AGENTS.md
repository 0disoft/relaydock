# AGENTS.md

## Repository Purpose

This repository is not a general-purpose chatbot. It is a runtime that controls the protocol, routing, cost, and consultation-handoff boundaries of AI requests. Boundaries and failure semantics matter more than feature count.

## Mandatory Rules

### Wails

- Only `internal/desktopwails` may import Wails packages.
- Wails services are limited to DTO conversion and use-case invocation.
- Keep frontend binding calls behind a thin adapter.
- Do not treat closing the window as terminating the runtime.
- Treat the arguments and additional data from a second instance as untrusted input.

### MCP

- Never write anything except MCP messages to stdout on the STDIO transport.
- Send all logs to stderr.
- Tool handlers must return a consultation ID instead of waiting for long-running work to finish.
- Grant Codex tokens only create/read/cancel scopes, and ChatGPT connector tokens only read/answer scopes.
- Do not allow delegation depth greater than 1.

### Protocol Conversion

- Do not immediately discard unknown fields or events.
- Reject any conversion with loss in strict mode.
- Record every compatible-mode conversion in the loss report.
- Allow passthrough only when upstream and ingress share the same semantic contract.
- Never retry automatically after the first semantic event.

### Billing

- PostgreSQL is authoritative state.
- Use Valkey only for leases, rate limits, cooldowns, and affinity.
- Do not combine usage events and customer charges in the same row.
- Pin the price revision when the request begins.
- Record adjustments as adjustment entries instead of modifying original rows.
- Do not create a capture API without an idempotency key.

### Security

- Apply SSRF checks before connecting to arbitrary provider or MCP URLs.
- Select and redact ContextPack content locally before transmission.
- Disable prompt and response body logging by default.
- Do not ship ChatGPT web DOM automation, cookie extraction, or imitation of private endpoints as official features.
- Secret values must never appear in errors or structured logs.

### Code

- Do not edit generated directories directly.
- Do not disguise TODOs as successful behavior.
- Paths that require external dependencies must return explicit configuration errors; use local in-memory implementations to verify vertical flows.
- Public interface changes require an ADR or contract-document update.
- Centralize state transitions in one state machine instead of scattering them across switch statements.
- Do not embed provider-specific exceptions directly in the canonical core.

## Generated Directories

```text
frontend/bindings/
gen/go/
internal/persistence/postgres/sqlcgen/
```

## Test Completion Criteria

Add at least one applicable test for the changed area:

- golden wire fixture
- state transition test
- fuzz test
- fault injection test
- billing idempotency test
- Playwright UI test
- migration round-trip test

## Review Priorities

1. Double charging, privilege escalation, and secret disclosure
2. Duplicate execution after a partial stream
3. Lossy protocol conversion
4. State-machine bypass
5. Missing cancellation or timeout handling
6. Unbounded memory, body size, or concurrency
7. Unobservable failures
8. Maintainability
