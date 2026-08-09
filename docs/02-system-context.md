# 02. System Context

```text
Codex / Claude Code / OpenCode
              | MCP stdio
              v
        mcp-bridge
              | Named Pipe / UDS
              v
+-----------------------------------+
| Wails v3 Desktop Runtime          |
|                                   |
| Local credential store            |
| Provider connections              |
| ContextPack compiler              |
| Secret redactor                   |
| Consultation client               |
| Runtime status                    |
+--------------+--------------------+
               | HTTPS / Connect
               v
+-----------------------------------+
| Expert Broker                     |
| consultation state                |
| API expert route                  |
| ChatGPT web handoff               |
+--------------+--------------------+
               |
               v
        Official model APIs

Applications
    | OpenAI / Anthropic / Gemini ingress
    v
+-----------------------------------+
| Gateway Data Plane                |
| auth -> compile -> route -> stream|
+-------+-------------------+-------+
        |                   |
        v                   v
 Official APIs          Self-hosted models
        |
        v
 Usage events ---------------------> money-platform

Control Plane -- signed snapshots -> Gateway
```

## Trust Boundaries

### Desktop

Keep personal OAuth tokens and API keys inside the desktop boundary. Send only status and capabilities to the server.

### Gateway

The Gateway handles customer requests but does not own authoritative customer balances.

### Expert Broker

The Expert Broker may retain ContextPacks for a limited time but has no repository access by default.

### Money Platform

The money-platform owns balances, holds, captures, releases, and adjustments. Usage rows in the Gateway database are not authoritative money state.
