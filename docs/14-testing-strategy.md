# 14. Testing Strategy

## Golden Streams

Store raw provider events as JSONL fixtures and compare them with canonical events and client output.

Required fixtures:

- Plain text
- Reasoning
- Parallel tool calls
- Fragmented tool JSON
- Image
- Refusal
- Unknown event
- 429 before streaming
- Disconnect before a semantic event
- Disconnect after a semantic event
- Cancellation

## State Machines

Verify consultation, request, attempt, stream, and charge transitions with table-driven tests.

## Fuzzing

- SSE parser
- JSON-fragment assembler
- Virtual-key parser
- URL validator
- Redaction scanner
- Provider-extension preservation

## Fault Injection

- PostgreSQL commit timeout
- Valkey loss
- Provider half-close
- Slow-client backpressure
- Partial object-store upload
- Duplicate webhook
- Process restart during capture

## UI

Playwright replaces Wails bindings with a mock adapter and verifies:

- Context preview
- Approval
- Consultation progress
- Stale-result warning
- Provider-degraded state
- Update-rollback message
