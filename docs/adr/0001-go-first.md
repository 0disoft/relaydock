# ADR-0001: Go-First Runtime

- Status: Accepted
- Date: 2026-08-06
- Owners: Runtime, Desktop, Control Plane

## Context

The product centers on long-lived connections, SSE and NDJSON streams, cancellation propagation, provider-specific HTTP connection pools, a local daemon, MCP STDIO, and operational servers. Adding Node and Rust for UI convenience would duplicate the same protocol, state-machine, and security rules across languages.

## Decision

Implement the Gateway, Control Plane, Expert Broker, headless runtime, MCP bridge, and Wails desktop backend in Go. Use TypeScript only for the Svelte UI, generated API types, and browser-input validation. Keep Python out of operational request paths; allow it only for evaluation fixtures and offline analysis.

Core domain packages do not import Wails, HTTP frameworks, or database drivers. Isolate external technologies in adapters under `internal/desktopwails`, `internal/persistence`, and `internal/transport`.

## Consequences

- Desktop, server, and headless runtimes share the protocol compiler and ContextPack logic.
- The project uses one Go toolchain and build cache.
- UI-only logic must not be pushed into Go services.
- Do not add provider-specific scripting runtimes merely because Go is inconvenient.

## Validation

`go test -race ./internal/... ./tests/...`, stream golden tests, and MCP bridge integration tests must pass. A new runtime language requires a separate ADR and analysis of deployment, observability, and security costs.
