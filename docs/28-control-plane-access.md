# 28. Control Plane Access

## Boundary

Control Plane authentication establishes a principal. The `control-access/v1` server policy separately decides whether that principal may read models, consume signed snapshots, or publish a new global revision. Client UI state is never an authorization boundary.

Health and readiness remain public. Every `/v1/*` route requires an authenticated principal unless `controld` is bound to loopback with no credentials, in which case it creates an explicit `loopback-development` administrator. Binding outside loopback still fails at startup without configured credentials.

## Roles

| Role | Scope | Allowed actions |
|---|---|---|
| `gateway` | Cluster only | Read/watch signed snapshots, read the signing key, read models |
| `viewer` | Cluster, tenant, or project | Read models only |
| `publisher` | Cluster only | Read Control data and publish a new snapshot revision |
| `admin` | Cluster only | Every action currently defined by `control-access/v1` |

Unknown roles and unknown actions deny by default. A future action is not automatically granted to an existing role.

Tenant- and project-scoped viewers receive only model routes allowed by matching virtual keys in the current signed snapshot. Responses remove internal provider account IDs. Scoped viewers cannot read the full signed snapshot because it contains cluster policy and virtual-key metadata, and they cannot watch or publish global revisions.

## Static Bootstrap Credentials

`CONTROL_ACCESS_TOKENS_JSON` is a strict JSON array. It stores only SHA-256 token digests, never raw bearer tokens.

```json
[
  {
    "tokenSha256": "<64-lowercase-hex-characters>",
    "subject": "gateway-seoul-01",
    "role": "gateway"
  },
  {
    "tokenSha256": "<64-lowercase-hex-characters>",
    "subject": "project-a-viewer",
    "role": "viewer",
    "tenantId": "00000000-0000-0000-0000-000000000001",
    "projectId": "00000000-0000-0000-0000-000000000002"
  }
]
```

Generate a random token and its digest locally. Store the raw token in the client secret store and copy only the digest into the server configuration.

```powershell
$tokenBytes = [Security.Cryptography.RandomNumberGenerator]::GetBytes(32)
$token = [Convert]::ToBase64String($tokenBytes)
$digestBytes = [Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($token))
$tokenDigest = ([Convert]::ToHexString($digestBytes)).ToLowerInvariant()
```

`CONTROL_BEARER_TOKEN` remains a compatibility path and maps to a cluster `admin`. Do not share it between Gateway, operators, or viewers. Migrate production deployments to separate digested credentials before enabling OIDC.

Clients continue to send the raw token in `Authorization: Bearer <token>`. `GATEWAY_CONTROL_BEARER_TOKEN` is the Gateway-side secret and should correspond to a server credential with role `gateway`. Operators may provide a separate publisher token through `CONTROL_BEARER_TOKEN` to `controlctl`; this client-side variable does not need to match the server's legacy compatibility variable.

## Audit Contract

Every authorization decision records only safe metadata:

- subject and role
- tenant and project IDs when scoped
- action
- allow or deny
- stable denial reason
- policy version

Authentication failures record no bearer token or token digest. Permission failures return `403 permission_denied`; missing or invalid identity returns `401 authentication_required`. Responses expose `X-RelayDock-Policy-Version` for authorization decisions.

## OIDC Handoff

Static credentials are a bootstrap and service-to-service mechanism, not the final human login system. The next implementation unit must verify OIDC issuer, audience, signature, subject, expiry, not-before, nonce and PKCE where applicable, then map trusted identity to the same `Principal` and `control-access/v1` policy. OIDC provider scopes must not become RelayDock roles directly.
