# ADR-0002: Wails v3 Desktop

- Status: Accepted with risk
- Date: 2026-08-06
- Owners: Desktop Runtime

## Context

The desktop app must combine a Go local runtime, system tray, single-instance handling, autostart, a native window, and a Svelte UI. Adopting Tauri would add a Rust shell and Cargo build graph on top of the existing Go runtime.

## Decision

Use Wails v3 instead of Tauri. Allow Wails imports only in `internal/desktopwails` and the root executable entrypoint. Wails services contain thin use-case calls only, with no protocol, routing, or accounting logic.

The desktop process is a per-user tray agent. Login autostart passes `--background` to start IPC with the window hidden. Environments without a GUI use the separate `cmd/headless` executable.

## Risks and Controls

- Limit v3 prerelease API risk with an exact pin and canary upgrades.
- Validate window closing, sleep/wake, WebView2 recreation, single-instance behavior, and the updater on physical platform devices.
- Do not expose unserializable interfaces or `context.Context` arguments from public Wails service methods.
- Never use Wails state as the domain source of truth.

## Exit Strategy

If Wails must be replaced, change only `internal/desktopwails` and build assets. Preserve the local IPC, ContextPack, provider-routing, and consultation-store contracts.
