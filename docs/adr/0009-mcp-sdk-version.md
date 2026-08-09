# ADR-0009: MCP Go SDK Version

- Status: Accepted
- Date: 2026-08-06
- Owners: MCP

## Context

The MCP SDK determines transport, tool schema, progress, cancellation, and authorization behavior. Following `latest` can change Codex connectivity or generated schemas without warning in every release.

## Decision

Pin the Go SDK to `v1.6.1`. The MCP STDIO bridge and Remote MCP server use the same SDK major and minor. Do not expose SDK types to the domain layer; use `internal/mcpcontract` DTOs at the boundary.

## Upgrade Policy

Promote a new version only after a separate branch passes:

- Initialization and capability negotiation
- Tool list and JSON-schema snapshot
- Malformed requests and oversized frames
- Cancellation, deadlines, and progress
- Stdout-contamination prevention
- A real Codex connection and Remote MCP authorization

Do not upgrade based only on protocol-date support. Server/client behavior and compatibility of existing tool names take priority.
