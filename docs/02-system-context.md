# 02. System Context

```text
Codex / Claude Code / OpenCode
              │ MCP stdio
              ▼
        mcp-bridge
              │ Named Pipe / UDS
              ▼
┌───────────────────────────────────┐
│ Wails v3 Desktop Runtime          │
│                                   │
│ Local credential store            │
│ Provider connections               │
│ ContextPack compiler               │
│ Secret redactor                    │
│ Consultation client                │
│ Runtime status                     │
└──────────────┬────────────────────┘
               │ HTTPS / Connect
               ▼
┌───────────────────────────────────┐
│ Expert Broker                     │
│ consultation state                │
│ API expert route                  │
│ ChatGPT web handoff               │
└──────────────┬────────────────────┘
               │
               ▼
        Official model APIs

Applications
    │ OpenAI / Anthropic / Gemini ingress
    ▼
┌───────────────────────────────────┐
│ Gateway Data Plane                │
│ auth → compile → route → stream   │
└───────┬───────────────────┬───────┘
        │                   │
        ▼                   ▼
 Official APIs          Self-hosted models
        │
        ▼
 Usage events ──────────────▶ money-platform

Control Plane ─ signed snapshots ─▶ Gateway
```

## 신뢰 경계

### Desktop

개인 OAuth와 API key는 desktop 경계 안에 둔다. 서버에는 상태와 capability만 올린다.

### Gateway

고객 요청을 처리하지만 고객 잔액의 권위 상태를 소유하지 않는다.

### Expert Broker

ContextPack을 제한된 시간 동안 보관할 수 있으나 원본 저장소 접근권을 기본으로 가지지 않는다.

### Money Platform

잔액, hold, capture, release, adjustment를 소유한다. Gateway DB의 usage row는 돈의 권위 상태가 아니다.
