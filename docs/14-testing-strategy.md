# 14. Testing Strategy

## Golden streams

provider raw event를 JSONL fixture로 보관하고 canonical event와 client output을 비교한다.

필수 fixture:

- plain text
- reasoning
- parallel tool call
- fragmented tool JSON
- image
- refusal
- unknown event
- 429 before stream
- disconnect before semantic
- disconnect after semantic
- cancellation

## State machine

consultation, request, attempt, stream, charge 상태 전이를 table test로 검증한다.

## Fuzz

- SSE parser
- JSON fragment assembler
- virtual key parser
- URL validator
- redaction scanner
- provider extension preservation

## Fault injection

- PostgreSQL commit timeout
- Valkey loss
- provider half-close
- slow client backpressure
- object store partial upload
- duplicate webhook
- process restart during capture

## UI

Playwright는 Wails binding을 mock adapter로 교체해 다음을 확인한다.

- context preview
- approval
- consultation progress
- stale result warning
- provider degraded state
- update rollback message
