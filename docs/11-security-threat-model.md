# 11. Security Threat Model

## Primary Assets

- Provider credentials
- Virtual API keys
- Tenant policies
- ContextPacks
- Consultation results
- Usage events
- Money authorization
- Update-signing keys
- Local IPC capabilities

## Attack Surfaces

### Local IPC

Another local user may read consultations or invoke tools.

Controls:

- Windows user-scoped Named Pipe ACL
- Unix socket mode 0600
- Nonce and peer identity
- Request-size limit
- Method allowlist

### Provider URL

A user-configured endpoint may attack a metadata service or private network.

Controls:

- Scheme allowlist
- Resolve DNS, then inspect the IP
- Recheck immediately before connecting
- Recheck every redirect
- Block private ranges by default
- Port policy

### Server Secret Resolution

A compromised proxy, malformed reference, redirect, or overbroad workload identity may expose provider credentials.

Controls:

- Lowercase scheme and fixed resource-shape validation
- Fixed metadata and Secret Manager origins
- No environment proxy for metadata-token requests
- No redirects and bounded response bodies
- Short-lived attached-workload tokens; no service-account key files
- Secret-level `secretmanager.versions.access`
- CRC32C payload verification
- Secret-free errors and fail-closed startup

### Remote MCP

A malicious web origin or oversized body may invoke tools.

Controls:

- OAuth scopes
- Origin and host validation
- Body limits
- Rate limits
- Separate read and answer tokens

### ContextPack

Repository secrets or customer data may be sent to an external model.

Controls:

- Local selection
- Redaction preview
- Explicit approval
- Encrypted objects
- Short TTL
- Digest audit

### Updater

An attacker may replace an artifact.

Controls:

- Signed manifest
- Artifact hash
- Code signing
- Rollback
- Canary channel
