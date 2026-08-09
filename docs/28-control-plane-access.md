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

`CONTROL_BEARER_TOKEN` remains a compatibility path and maps to a cluster `admin`. Do not share it between Gateway, operators, or viewers. Migrate production deployments to separate digested credentials or OIDC identities.

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

## OIDC ID-Token Bearers

`controld` can verify ID tokens from one operator-configured OpenID Connect issuer. Discovery must succeed at startup, the discovery document must report the exact configured issuer, and every bearer must pass signature, audience, authorized-party (`azp`), expiry, and not-before validation. A multiple-audience token requires `azp` to equal `CONTROL_OIDC_AUDIENCE`; a present `azp` must always match. The long-lived verifier caches JWKS keys and refreshes them when a new key ID appears.

```dotenv
CONTROL_OIDC_ISSUER=https://identity.example.com/realms/relaydock
CONTROL_OIDC_AUDIENCE=relaydock-control
CONTROL_OIDC_ROLE_MAPPINGS_JSON={"relaydock-viewer":"viewer","relaydock-publisher":"publisher","relaydock-admin":"admin"}
CONTROL_OIDC_ROLE_CLAIM=relaydock_role
CONTROL_OIDC_TENANT_CLAIM=relaydock_tenant_id
CONTROL_OIDC_PROJECT_CLAIM=relaydock_project_id
CONTROL_OIDC_HTTP_TIMEOUT=10s
```

`CONTROL_OIDC_ROLE_MAPPINGS_JSON` is mandatory when OIDC is enabled. The configured role claim must contain one exact mapping key. Ordinary `scope`, group, or provider-role claims never become RelayDock roles implicitly. The mapped principal is validated by the same rules as static credentials: only `viewer` may carry tenant/project scope, and `projectId` requires `tenantId`. The audit subject is a stable SHA-256 pseudonym of issuer plus OIDC subject; raw tokens and raw identity subjects are not logged.

Issuer discovery, redirects, and the discovered `jwks_uri` require HTTPS and reject userinfo, query strings, fragments, localhost names, private addresses, link-local ranges, and cloud metadata ranges. Connections dial a previously validated address instead of resolving the host a second time. `CONTROL_OIDC_ALLOW_PRIVATE_ISSUER=true` additionally allows numeric private and loopback addresses for controlled development; it does not allow localhost aliases, link-local addresses, or metadata endpoints.

Static credentials and OIDC may be enabled together. This preserves digested Gateway/bootstrap credentials while humans or automation use short-lived OIDC tokens. An OIDC discovery failure is a startup error rather than a silent fallback.

This boundary validates an already-issued ID token; it is not a browser login session. The Control Console still needs a backend-for-frontend authorization-code flow with state, nonce, PKCE, secure session cookies, logout, and membership/revocation storage. Do not expose ID tokens to browser storage or treat this API bearer contract as that future session implementation.
