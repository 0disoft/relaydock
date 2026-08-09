# Control Console

This SvelteKit server application reads the managed Control Plane. Server `load` functions read Control API status and model routes so the browser never receives the control-plane bearer token directly.

## Environment Variables

```text
CONTROL_API_BASE_URL=http://127.0.0.1:8081
CONTROL_BEARER_TOKEN=
```

The current UI provides read-only status, snapshot revision, expiration time, and virtual-model routes. Add organization, project, virtual-key mutations later as separate route actions after authentication and audit-log contracts are connected.

## Run

```bash
bun install
bun run dev
```

Never send raw provider secrets to the browser. This application does not own payment or balance ledgers; those remain delegated to the money-platform.
