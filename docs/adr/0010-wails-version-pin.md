# ADR-0010: Wails Version Pin

- Status: Accepted
- Date: 2026-08-06
- Owners: Desktop Build

## Context

Wails v3 is a rapidly changing prerelease whose CLI, Go module, TypeScript runtime, and generated build assets move together. A current CLI paired with an older Go module can break bindings and packaging.

## Decision

Pin the Wails Go module and CLI to `v3.0.0-alpha2.119`. Do not use `@latest` in CI, bootstrap scripts, or the Taskfile. If `@wailsio/runtime` is added, pin it to the same release line.

## Upgrade Procedure

1. Change the Go module, CLI, and runtime package together on a canary branch.
2. Review the `wails3 update build-assets` diff manually.
3. Regenerate bindings and check for unnecessary public-method exposure.
4. Verify tray, close-to-tray, `--background`, and single-instance behavior on Windows, macOS, and Linux.
5. Verify installation, update, rollback, and preservation of the MCP bridge path.
6. Record regression results and the reason for the change in a new ADR or an amendment to this ADR.

Do not update the production pin automatically before validation, even when a release appears to contain a security patch.
