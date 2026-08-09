# 09. Routing

## Stage 1: Hard Filter

- Protocol capability
- Context and output limits
- Modality
- Tool calling
- Structured output
- Reasoning preservation
- State continuation
- Region
- Tenant allowlist
- Maximum estimated cost

## Stage 2: Deterministic Score

```text
health
+ quota
+ latency
+ throughput
+ cache affinity
+ session affinity
+ configured weight
- estimated cost
- recent error
- queue pressure
```

## Virtual Model

Users request stable IDs such as `code-fast`, `code-deep`, and `vision-balanced`. The actual provider mapping is revisioned.

## Lease

Acquire a short lease so the concurrency slot cannot disappear between the route decision and the actual call.

When Valkey is unavailable, each provider's lease policy explicitly chooses fail-open or fail-closed behavior. This is unrelated to customer balances.
