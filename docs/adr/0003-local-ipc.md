# ADR-0003: Local IPC

- Status: Accepted
- Date: 2026-08-06
- Owners: Desktop Runtime, MCP

## Context

Codex launches an MCP STDIO process for each session, while provider state and consultation history must be shared in a long-running per-user runtime. Localhost TCP introduces port conflicts, firewall exposure, DNS rebinding, and an unnecessary authentication surface.

## Decision

The MCP bridge communicates with the desktop or headless runtime through a Windows Named Pipe or Unix Domain Socket. Messages use size-limited, length-prefixed JSON frames. The bridge only translates STDIO JSON-RPC and internal IPC; it holds no domain logic or credentials.

## Security Rules

- Unix sockets use a user-only directory and mode `0600`.
- Verified ACLs for the current user on Windows pipes are a release condition.
- Disconnect without executing the request when a frame exceeds the maximum size.
- Reserve stdout for MCP protocol traffic and send logs only to stderr.
- Do not include tenant IDs, tokens, or repository paths in endpoint names.

## Failure Handling

When the runtime is unavailable, the bridge returns a clear connection error. It must not claim success or secretly create a temporary independent runtime. Classify partial frames, EOF, deadlines, and cancellation as distinct errors.
