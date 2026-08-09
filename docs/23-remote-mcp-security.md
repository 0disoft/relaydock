# 23. Remote MCP Security

## Threat Model

Remote MCP can read redacted ContextPacks and read or write expert results, so it needs a stronger boundary than a health API. Sharing one long-lived bearer with the Broker administrative API would let one token disclosure modify consultations across every tenant.

## Credential Types

```text
Broker bearer
  Expert administrative HTTP API

Scoped MCP token
  consultations:read
  consultations:answer
  consultations:admin

Legacy MCP bearer
  cluster-wide credential for local compatibility
```

When `EXPERT_MCP_TOKEN_SECRET` or `_B64` is configured, scoped HMAC tokens are the default. The Broker then does not reuse `EXPERT_BROKER_BEARER_TOKEN` for MCP. Set `EXPERT_MCP_ALLOW_BROKER_TOKEN=true` only when compatibility requires it.

## Token Issuance

```powershell
go run ./cmd/mcptokenctl issue `
  --subject chatgpt-reviewer `
  --tenant <organizationId> `
  --project <projectId> `
  --scopes consultations:read,consultations:answer `
  --ttl 1h
```

Tokens contain audience, token ID, subject, tenant, project, scopes, issued-at, and expiry, and are HMAC-signed. Reject lifetimes beyond `EXPERT_MCP_TOKEN_MAX_LIFETIME`. `mcptokenctl` accepts only the three known scopes, requires both tenant and project for read/answer tokens, and never combines cluster-wide `consultations:admin` with other scopes. Reject noncanonical base64url claims or signatures to prevent multiple string forms of one token.

## Tenant Isolation

A scoped credential with a token ID must match both the consultation tenant and project. A mismatch between a ContextPack and its consultation is stored-state corruption. Guessing a consultation ID cannot grant access to another project's pack or permit result submission.

`consultations:admin` and legacy static tokens are cluster-wide. Do not issue them for normal ChatGPT handoffs or team reviewers.

## Tool Permissions

| MCP tool | Required scope |
|---|---|
| `consultation_get` | `consultations:read` |
| `consultation_submit_result` | `consultations:answer` |

Do not grant result-submission tokens to read-only connections. The Codex bridge performs create/read/cancel through local IPC, while a ChatGPT reviewer has read/answer only.

## HTTP Controls

- Place remote deployments behind TLS.
- Restrict the Host header with `AllowedHosts`.
- Set `AllowedOrigins` explicitly for browser connections.
- Limit request-body size.
- Never expose unauthenticated MCP outside loopback.
- Review trusted reverse-proxy settings when a proxy changes the original Host or Origin.

## Attestation

Remote MCP submitters provide `modelAttestation`, but it is not provider-signed model proof. Store `remote_mcp` provenance and the submitter together. Treat models selected by users in ChatGPT as user-declared, not equivalent to official API response attestation.

## Rotation and Retirement

New tokens use `argt2.<key-id>.<claims>.<hmac>`. `mcptokenctl` outputs the active `keyId`. The server issues only with the active secret and also verifies retiring secrets from `EXPERT_MCP_TOKEN_VERIFICATION_KEYS_B64`.

```powershell
$env:EXPERT_MCP_TOKEN_KEY_ID = "2026-q3"
$env:EXPERT_MCP_TOKEN_SECRET_B64 = "<new-base64-secret>"
$env:EXPERT_MCP_TOKEN_VERIFICATION_KEYS_B64 = '{"2026-q2":"<old-base64-secret>"}'
```

Because argt1 has no key ID, verify it against all migration-period secrets. After `EXPERT_MCP_TOKEN_MAX_LIFETIME` plus clock skew has elapsed since the last argt1 issuance, set `EXPERT_MCP_TOKEN_ALLOW_LEGACY=false`. Never reuse an active key ID with a different secret.
