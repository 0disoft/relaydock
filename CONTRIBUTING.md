# Contributing

Unless a separate written agreement states otherwise, contributions intentionally submitted to RelayDock are provided under the Apache License 2.0 terms in the root `LICENSE`.

## Branches

- `main`: code that passes tests and contract validation
- `feat/<area>-<name>`
- `fix/<area>-<name>`
- `adr/<number>-<name>`

## Before Committing

```powershell
./scripts/check.ps1
```

On Linux and macOS, use:

```bash
./scripts/check.sh
```

## Required PR Information

- Changed trust boundaries and data owners
- Failure scenarios reviewed before the happy path
- Whether database, event, or public API migration is required
- Effects on secrets, permissions, SSRF defenses, and stream retries
- Effects on billing and idempotency
- Added automated tests and manual verification
- Rollback conditions and procedure

## Generated Code

Update the outputs of `buf generate`, `sqlc generate`, and `wails3 generate bindings` together with their source contracts. Pull requests that modify only generated files are not accepted.

## Definition of Done

A new feature is not complete with only one happy path. Test every applicable failure path: cancellation, timeout, duplicate requests, partial streams, invalid state transitions, and size limits. When external providers or operational infrastructure are unavailable, verify semantics with an in-memory adapter or contract test and document the unverified scope.
