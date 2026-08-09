# Desktop Frontend

This Svelte SPA is embedded in the Wails v3 webview. It does not call network APIs directly; it accesses Go services through generated Wails bindings.

## Boundaries

- Import generated bindings only in `src/lib/runtime-adapter.ts`.
- Do not store provider keys or OAuth tokens in a Svelte store or `localStorage`.
- Display IDs for long-running work and refresh state through polling or Wails events.
- `dist/index.html` is the minimal fallback that keeps Go embedding valid. A real frontend build replaces the entire `dist/` directory.
