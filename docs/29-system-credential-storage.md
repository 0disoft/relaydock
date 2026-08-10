# 29. System Credential Storage

## Boundary

Provider API credentials belong in a per-user operating-system credential store, not in RelayDock settings, local state JSON, logs, or browser storage. `credentials.Store` remains the application port. `SystemStore` is the native desktop adapter and `MemoryStore` remains test/ephemeral-only.

Windows uses Credential Manager through `CredWriteW`, `CredReadW`, `CredDeleteW`, and `CredFree`. macOS uses Security.framework generic-password items through `SecItemAdd`, `SecItemUpdate`, `SecItemCopyMatching`, and `SecItemDelete`. Linux uses the freedesktop Secret Service API over the current user's D-Bus session bus. All three backends use the current user's operating-system credential boundary, and none has a plaintext-file fallback.

The macOS backend links Security.framework only in Darwin builds. It does not invoke `/usr/bin/security`, because its password option would place credential material in process arguments. Go-owned write values are copied into C-owned buffers, and returned C buffers are bounded and explicitly zeroed before release. Keychain errors expose only the operation and numeric `OSStatus`, never item data.

The Linux backend opens a Secret Service `plain` session, searches by a fixed RelayDock application attribute plus the opaque target, and writes to the user's `default` collection with replacement enabled. It follows `Unlock` and `Prompt` results for locked collections and items. Missing services or collections fail closed. Duplicate matching items, malformed object paths, unexpected session data, and dismissed prompts never select or expose an arbitrary credential. Remote D-Bus error bodies are omitted from RelayDock errors.

The `plain` Secret Service algorithm does not add application-layer encryption. Its trust boundary is the authenticated per-user D-Bus session and the Secret Service implementation. RelayDock does not place secret material in process arguments, environment variables, labels, attributes, or logs. Deployments that permit untrusted same-user processes must evaluate the desktop session boundary separately.

The desktop provider page now exposes status, save, replace, and delete operations through the Wails runtime service. The frontend never receives a stored credential value. Submitted values use password inputs and are cleared after every success or failure. Credential changes are denied while the local Gateway is starting or running so a displayed state cannot diverge from the active runtime.

Environment credentials retain precedence and are read-only in the desktop UI. When no environment override exists, Gateway composition resolves the provider key from the system store. A stored real-provider credential disables the implicit `local/echo` fallback in the same way as an environment credential. Store errors fail closed and never trigger a plaintext fallback.

## Identity and Size Contract

Each credential reference contains a provider, account, and logical ID. The operating-system target is:

```text
<stable-namespace>:base64url(sha256(provider + NUL + account + NUL + id))
```

The target does not expose provider, account, or logical ID in the Credential Manager list. This is metadata minimization, not an authorization boundary: another process running as the same user can still access a known target through the same operating-system API.

Credential values must contain between 1 and 2,048 bytes. This stays below the Windows generic-credential blob limit and covers ordinary provider API keys. Callers and backends exchange copies; transient adapter copies are zeroed after writes and reads. Errors never include credential bytes. The opaque target is stored as the macOS generic-password service and as a Linux Secret Service attribute; neither reveals provider, account, or logical credential identity.

## Failure Contract

- Missing reads return `core.ErrNotFound`.
- Deleting a missing credential succeeds, matching the in-memory store contract.
- Invalid references, namespaces, empty values, and oversized values fail before an operating-system call.
- A missing D-Bus session, Secret Service, default collection, or native backend returns `ErrSystemStoreUnavailable`; RelayDock must not silently downgrade to a plaintext file.
- Context cancellation is propagated through Linux D-Bus calls and prompt waits. Windows Credential Manager and macOS Keychain operations are not cancellable once entered, so callers must not report a timed-out write as definitely absent.

## Verification Boundary

The common adapter tests cover opaque targets, caller-memory isolation, size and namespace bounds, cancellation, idempotent deletion, missing values, and secret-free errors. Secret Service tests additionally cover session negotiation, default-collection replacement, opaque attributes, locked-item prompts, duplicate rejection, invalid paths, buffer zeroing, and remote error-body omission. Windows CI compiles the complete desktop path, Linux CI runs the D-Bus protocol tests, and a dedicated macOS job compiles Security.framework integration. Automated suites intentionally do not write real credentials into developer or hosted-runner accounts; disposable-user physical-device smoke tests remain required before production certification.

Windows Credential Manager, macOS Keychain, and Linux Secret Service are desktop-user boundaries and must not be presented as clustered server secret-management solutions.

## Server Workload Identity

`gatewayd` supports read-only server-secret references for provider API keys and the virtual-key pepper. The server resolver is separate from `credentials.Store`: it cannot create, replace, or delete a secret, and desktop composition never enables it. Direct provider-key environment variables keep highest priority, the desktop system store remains second when present, and a server reference is consulted only when neither source supplied a value.

The first production adapter is Google Cloud Secret Manager with an attached workload identity. References use this canonical form:

```text
gcp-sm:projects/PROJECT_ID/secrets/SECRET_ID/versions/VERSION_ID
```

`VERSION_ID` may be a numeric version, `latest`, or a configured alias. `gatewayd` accepts the following reference variables:

- `GATEWAY_OPENAI_API_KEY_REF`
- `GATEWAY_ANTHROPIC_API_KEY_REF`
- `GATEWAY_GOOGLE_API_KEY_REF`
- `GATEWAY_DEEPSEEK_API_KEY_REF`
- `GATEWAY_OPENROUTER_API_KEY_REF`
- `GATEWAY_OPENAI_COMPATIBLE_API_KEY_REF`
- `GATEWAY_VIRTUAL_KEY_PEPPER_REF`

The adapter requests a short-lived OAuth access token from the fixed Google metadata endpoint with `Metadata-Flavor: Google`, then accesses the fixed HTTPS Secret Manager v1 endpoint. Metadata requests bypass environment proxies, and neither endpoint is configurable in production. Redirects are rejected. Token and response bodies are bounded, non-success bodies are never copied into errors, and returned secret bytes must pass the Secret Manager CRC32C check. Provider credentials retain the common 1-to-2,048-byte size and surrounding-whitespace rules. A resolved virtual-key pepper must contain 32 to 4,096 raw bytes. `GATEWAY_VIRTUAL_KEY_PEPPER_B64` takes precedence over `GATEWAY_VIRTUAL_KEY_PEPPER`, which takes precedence over `GATEWAY_VIRTUAL_KEY_PEPPER_REF`; `gatewayd` and `keyctl` use the same resolution contract.

Use an immutable numeric Secret Manager version for the pepper. Changing the resolved bytes immediately makes every existing virtual key unverifiable, so `latest` and mutable aliases are unsafe unless the deployment intentionally coordinates a full key replacement. RelayDock does not retain an old-pepper verification ring.

The attached service account or GKE workload identity needs `secretmanager.versions.access`, normally through `roles/secretmanager.secretAccessor`, granted on each required secret rather than on the whole project. Compute Engine and GKE nodes must expose the `cloud-platform` OAuth scope required by Secret Manager. RelayDock does not accept service-account key files or long-lived Google credentials for this path.

Missing workload identity, denied IAM access, malformed references, unavailable metadata, redirects, corrupt payloads, and unsupported reference schemes fail Gateway startup. Removing a reference and restoring the corresponding direct environment variable is the rollback path. Other server secrets, including database URLs, Control signing keys, MCP HMAC keys, and webhook secrets, still require deployment-platform secret injection or future adapters; this release must not be described as complete KMS coverage.
