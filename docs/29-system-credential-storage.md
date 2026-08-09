# 29. System Credential Storage

## Boundary

Provider API credentials belong in a per-user operating-system credential store, not in RelayDock settings, local state JSON, logs, or browser storage. `credentials.Store` remains the application port. `SystemStore` is the native desktop adapter and `MemoryStore` remains test/ephemeral-only.

Windows uses Credential Manager through `CredWriteW`, `CredReadW`, `CredDeleteW`, and `CredFree`. macOS uses Security.framework generic-password items through `SecItemAdd`, `SecItemUpdate`, `SecItemCopyMatching`, and `SecItemDelete`. Both backends use the current user's operating-system credential boundary, and neither has a plaintext-file fallback. Linux currently returns `ErrSystemStoreUnavailable` until a native Secret Service adapter is implemented.

The macOS backend links Security.framework only in Darwin builds. It does not invoke `/usr/bin/security`, because its password option would place credential material in process arguments. Go-owned write values are copied into C-owned buffers, and returned C buffers are bounded and explicitly zeroed before release. Keychain errors expose only the operation and numeric `OSStatus`, never item data.

The desktop provider page now exposes status, save, replace, and delete operations through the Wails runtime service. The frontend never receives a stored credential value. Submitted values use password inputs and are cleared after every success or failure. Credential changes are denied while the local Gateway is starting or running so a displayed state cannot diverge from the active runtime.

Environment credentials retain precedence and are read-only in the desktop UI. When no environment override exists, Gateway composition resolves the provider key from the system store. A stored real-provider credential disables the implicit `local/echo` fallback in the same way as an environment credential. Store errors fail closed and never trigger a plaintext fallback.

## Identity and Size Contract

Each credential reference contains a provider, account, and logical ID. The operating-system target is:

```text
<stable-namespace>:base64url(sha256(provider + NUL + account + NUL + id))
```

The target does not expose provider, account, or logical ID in the Credential Manager list. This is metadata minimization, not an authorization boundary: another process running as the same user can still access a known target through the same operating-system API.

Credential values must contain between 1 and 2,048 bytes. This stays below the Windows generic-credential blob limit and covers ordinary provider API keys. Callers and backends exchange copies; transient adapter copies are zeroed after writes and reads. Errors never include credential bytes. The opaque target is stored as the macOS generic-password service, with `RelayDock` as its fixed account label.

## Failure Contract

- Missing reads return `core.ErrNotFound`.
- Deleting a missing credential succeeds, matching the in-memory store contract.
- Invalid references, namespaces, empty values, and oversized values fail before an operating-system call.
- A missing native backend returns `ErrSystemStoreUnavailable`; RelayDock must not silently downgrade to a plaintext file.
- Context cancellation is honored before entering a synchronous operating-system call. Windows Credential Manager and macOS Keychain operations are not cancellable once entered, so callers must not report a timed-out write as definitely absent.

## Verification Boundary

The common adapter tests cover opaque targets, caller-memory isolation, size and namespace bounds, cancellation, idempotent deletion, missing values, and secret-free errors. Windows CI compiles the complete desktop path. A dedicated macOS job compiles Security.framework integration and runs the common credential tests. Automated suites intentionally do not write real credentials into developer or hosted-runner accounts; disposable-user physical-device smoke tests remain required before production certification.

Server processes require a separate KMS or workload-identity adapter. Windows Credential Manager and macOS Keychain are desktop-user boundaries and must not be presented as clustered server secret-management solutions.
