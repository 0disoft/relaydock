# 13. Observability

## Default Signals

- Request ID
- Tenant/project
- Ingress protocol
- Virtual model
- Provider/model
- Route revision
- Attempt
- TTFT
- First semantic event
- Inter-token latency
- Output TPS
- Cancellation latency
- Usage dimensions
- Estimated/provider cost
- Loss report
- Retry phase

## Sensitive Data

Do not include prompts, responses, tool arguments, or ContextPack content in default traces or logs.

## Important Metrics

- Authentication latency
- Snapshot lookup
- Route decision
- Provider queue wait
- Connection latency
- TTFT
- Partial-stream rate
- Pre-semantic retry rate
- Post-semantic failure rate
- Usage mismatch
- Capture conflict
- Stale ContextPack result
