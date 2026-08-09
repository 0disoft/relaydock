# 05. Expert Escalation

## Separation of Roles

```text
Expert
judgment, critique, alternatives, verification conditions

Coding Agent
file edits, command execution, tests, result comparison
```

Expert routes have no repository write access by default.

## Routes

### `openai_api_pro`

Automatically calls an official API. The provider response identifies the execution model and usage.

### `chatgpt_web_handoff`

Codex creates a consultation and ContextPack for the user to review directly in ChatGPT. The product does not automate the browser DOM.

### `workspace_agent`

An extension route to add after enterprise validation. It is excluded from the default MVP.

## Automatic Escalation Conditions

- Payments, authentication, or permissions
- Irreversible schema migration
- Breaking public API change
- Two failed fixes with different approaches
- Conflicting agent analyses
- Explicit user request

## Cost Controls

- Maximum calls per task
- Maximum estimated cost per task
- Delegation depth of 1
- Duplicate-execution prevention for the same ContextPack digest
- Approval policy
