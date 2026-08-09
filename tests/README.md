# Tests

Test directories follow product boundaries:

- `conformance`: ingress and egress protocol contracts
- `golden_streams`: provider-event replay and ordering verification
- `fault_injection`: midstream disconnects, cancellation, 429/5xx, and backpressure
- `billing`: usage idempotency and quote-hold-capture settlement
- `expert`: ContextPack, redaction, consultation state machine, and recursion prevention
- `load`: separate TTFT, inter-token latency, and queue-delay measurements

Live provider tests do not run in the default CI workflow. Use a separate nightly probe with restricted credentials.
