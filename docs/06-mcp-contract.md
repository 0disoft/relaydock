# 06. MCP Contract

## Local STDIO Tools

| Tool | Permission | Purpose |
|---|---|---|
| `expert_consultation_create` | create | Select repository context and create a consultation |
| `expert_consultation_get` | read | Read state and the complete structured result |
| `expert_consultation_cancel` | cancel | Cancel a consultation before completion |
| `context_pack_preview` | read-local | Preview candidate files and exclusions |

STDIO MCP has no approval or expert-result submission permission. The user approves in the Wails UI, and results arrive through an API route or a separate ChatGPT connector.

## Remote MCP Tools

| Tool | Permission | Purpose |
|---|---|---|
| `consultation_get` | read | Let ChatGPT read a consultation and ContextPack |
| `consultation_submit_result` | answer | Submit a structured expert result |

## Token Separation

```text
Codex token
create, read, cancel

ChatGPT connector token
read, answer
```

Do not grant both create and answer to one token. Remote MCP currently separates `consultations:read`, `consultations:answer`, and `consultations:admin` with HMAC-scoped tokens and enforces tenant/project matches. A legacy static bearer is a cluster-wide compatibility credential; the Broker does not automatically reuse its administrative bearer when a scoped-token secret is configured. Replace this with OIDC/OAuth and key-ID-based rotation for long-term operation.

## Local IPC

`cmd/mcp-bridge`, launched by Codex, contains no domain logic. It translates STDIO JSON-RPC into length-bounded JSON frames over a per-user Named Pipe or Unix Domain Socket. The desktop or headless runtime executes the actual ContextPack and consultation use cases.

## STDIO Rules

- Write only MCP JSON-RPC to stdout.
- Send all logs to stderr.
- Do not print startup banners or panic stacks to stdout.
- Do not wait for long-running model calls inside an STDIO handler.
- Set independent size limits for request bodies, IPC frames, and ContextPacks.
- Reject consultation creation when delegation depth exceeds 1.
