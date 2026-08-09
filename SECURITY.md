# Security Policy

## Report Privately

- Virtual-key bypass
- Tenant-boundary bypass
- Provider credential exposure
- ContextPack secret disclosure
- Local IPC permission bypass
- DNS rebinding or SSRF
- Forged usage or duplicate capture
- MCP tool-scope bypass
- Updater-signature bypass

Do not post tokens, cookies, raw prompts, customer data, or exploit payloads in public issues.

## Default Security Posture

- Fail closed for authentication, payment capture, secret redaction, and remote MCP writes.
- Never fail open for tenant scope, provider credentials, or the usage ledger.
- A last-known-good snapshot may be used only when the tenant has explicitly configured a snapshot-expiration policy.
