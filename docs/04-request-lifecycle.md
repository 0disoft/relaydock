# 04. Request Lifecycle

```text
authenticate
-> budget authorization
-> decode ingress
-> capability requirement extraction
-> candidate hard filter
-> route decision
-> account/provider lease
-> upstream request
-> stream translation
-> usage finalization
-> capture/release
-> reconciliation
```

## Retry Boundary

### Pre-Semantic Retry

No text, reasoning, tool call, or image chunk has been delivered to the user. The same request may be retried as a new attempt.

### Post-Semantic Failure

The first semantic event has already been delivered. Do not silently switch to another provider.

Allowed handling:

- Resume the same attempt when the provider officially supports resume.
- Expose a partial failure to the client.
- Create an explicit continuation approved by the user.

## Cancellation

Propagate client cancellation through every layer:

```text
client context
-> ingress handler
-> router lease
-> provider request
-> expert job
-> accounting provisional state
```

Cancellation does not erase provider costs already incurred. Reconcile usage and charge policies separately.
