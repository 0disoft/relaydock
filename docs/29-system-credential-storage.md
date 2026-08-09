# 29. System Credential Storage

## Boundary

Provider API credentials belong in a per-user operating-system credential store, not in RelayDock settings, local state JSON, logs, or browser storage. `credentials.Store` remains the application port. `SystemStore` is the native desktop adapter and `MemoryStore` remains test/ephemeral-only.

The first native backend supports Windows Credential Manager through `CredWriteW`, `CredReadW`, `CredDeleteW`, and `CredFree`. It uses a generic credential persisted for the current user on the local machine. No plaintext-file fallback exists. macOS and Linux currently return `ErrSystemStoreUnavailable` until native Keychain and Secret Service adapters are implemented.

The desktop provider page now exposes status, save, replace, and delete operations through the Wails runtime service. The frontend never receives a stored credential value. Submitted values use password inputs and are cleared after every success or failure. Credential changes are denied while the local Gateway is starting or running so a displayed state cannot diverge from the active runtime.

Environment credentials retain precedence and are read-only in the desktop UI. When no environment override exists, Gateway composition resolves the provider key from the system store. A stored real-provider credential disables the implicit `local/echo` fallback in the same way as an environment credential. Store errors fail closed and never trigger a plaintext fallback.

## Identity and Size Contract

Each credential reference contains a provider, account, and logical ID. The operating-system target is:

```text
<stable-namespace>:base64url(sha256(provider + NUL + account + NUL + id))
```

The target does not expose provider, account, or logical ID in the Credential Manager list. This is metadata minimization, not an authorization boundary: another process running as the same user can still access a known target through the same operating-system API.

Credential values must contain between 1 and 2,048 bytes. This stays below the Windows generic-credential blob limit and covers ordinary provider API keys. Callers and backends exchange copies; transient adapter copies are zeroed after writes and reads. Errors never include credential bytes.

## Failure Contract

- Missing reads return `core.ErrNotFound`.
- Deleting a missing credential succeeds, matching the in-memory store contract.
- Invalid references, namespaces, empty values, and oversized values fail before an operating-system call.
- A missing native backend returns `ErrSystemStoreUnavailable`; RelayDock must not silently downgrade to a plaintext file.
- Context cancellation is honored before entering the synchronous operating-system call. The Windows Credential Manager API itself is not cancellable, so callers must not report a timed-out write as definitely absent.

## Verification Boundary

The common adapter tests cover opaque targets, caller-memory isolation, size and namespace bounds, cancellation, idempotent deletion, missing values, and secret-free errors. The Windows implementation compiles and passes the full Go suite on Windows. The automated suite intentionally does not write a real credential into a developer or hosted-runner account; a disposable-user physical-device smoke remains required before production certification.

Server processes require a separate KMS or workload-identity adapter. Windows Credential Manager is a desktop-user boundary and must not be presented as a clustered server secret-management solution.
