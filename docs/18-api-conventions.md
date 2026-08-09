# 18. API Conventions

## IDs

Use opaque strings:

```text
req_
att_
use_
con_
ctx_
res_
snap_
vkey_
```

Do not couple database primary-key formats to API ID formats.

## Time

- UTC
- RFC 3339
- Durations as integer milliseconds or Protobuf Duration
- No relative-time strings

## Idempotency

Side-effecting endpoints require `idempotency_key`:

- Consultation creation
- Hold
- Capture
- Release
- Virtual-key rotation
- Provider-credential rotation

## Error Envelope

```json
{
  "code": "CONSULTATION_INVALID_STATE",
  "message": "The consultation cannot transition from completed to running.",
  "request_id": "req_...",
  "retryable": false,
  "details": {}
}
```

## Pagination

Use cursor-based pagination. Offset pagination is allowed only for small administrative lists.
