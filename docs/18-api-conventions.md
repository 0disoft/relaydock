# 18. API Conventions

## ID

opaque string을 쓴다.

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

DB PK 형식과 API ID 형식을 결합하지 않는다.

## Time

- UTC
- RFC 3339
- duration은 integer milliseconds 또는 protobuf Duration
- 상대 시간 문자열 금지

## Idempotency

side effect endpoint는 `idempotency_key`를 요구한다.

- consultation create
- hold
- capture
- release
- virtual key rotate
- provider credential rotate

## Error envelope

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

cursor 기반. offset pagination은 관리자 소량 목록 외에 금지한다.
