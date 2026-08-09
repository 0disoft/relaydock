# 22. Runtime Journal and Transactional Outbox

## 목적

Gateway의 사용자 요청, 공급자 재시도, 최종 사용량과 외부 정산 이벤트를 하나의 추적 가능한 lifecycle로 남긴다. 마지막으로 성공한 공급자 호출만 기록하면 실패한 attempt에서 실제 발생한 비용과 장애 원인을 잃기 때문에 request와 attempt를 분리한다.

## 데이터 모델

```text
runtime.requests
  1 ── N runtime.provider_attempts
  1 ── N runtime.usage_events
  1 ── N outbox.events
```

`runtime.requests`는 사용자 관점의 한 요청이다. `provider_attempts`는 retry·fallback마다 별도로 생성된다. usage는 공급자별 attempt에 귀속되며 request 완료 행에는 모든 attempt의 합계를 보존한다.

## lifecycle

```text
BeginRequest
  → BeginAttempt
  → optional MarkCommitted
  → FinishAttempt
  → zero or more retry attempts
  → FinishRequest + InsertOutboxEvent in one transaction
```

첫 semantic event가 전달된 시각은 `first_semantic_event_at`으로 기록한다. 이 경계 이후의 실패는 다른 공급자로 숨겨서 재시도하지 않으며, committed attempt로 남는다.

## idempotency

- 같은 request ID나 attempt ID의 동일 쓰기는 성공으로 처리할 수 있다.
- 같은 ID인데 tenant, model, 사용량, 상태, payload가 다르면 conflict다.
- outbox event ID는 downstream의 idempotency key다.
- event ID만 같고 payload가 다른 재삽입은 데이터 손상으로 거절한다.

## transaction 경계

`FinishRequest`와 request-completed outbox 생성은 같은 PostgreSQL transaction에서 실행한다. DB commit 뒤 별도 publish를 시도하는 방식은 process crash 순간에 정산 이벤트를 영구 유실하므로 금지한다.

## outbox worker

`outboxd`는 `FOR UPDATE SKIP LOCKED`로 batch를 claim하고 다음 정보를 저장한다.

```text
locked_by
locked_at
lock_expires_at
attempts
last_error
published_at 또는 dead_at
```

완료·retry·dead-letter·release는 worker ID뿐 아니라 처리 완료 시각이 lease 만료 전인지 검사한다. lease를 잃은 오래된 worker가 뒤늦게 성공을 기록하는 것을 fencing한다.

## webhook delivery

- 본문은 event envelope의 정확한 JSON이다.
- `Idempotency-Key`에 event ID를 넣는다.
- timestamp와 body를 HMAC-SHA256으로 서명한다.
- receiver는 허용 시간창, 미래 시각, body 변조를 검증한다.
- redirect는 따라가지 않는다.
- loopback이 아닌 평문 HTTP는 명시적 개발 옵션 없이는 거절한다.
- 2xx는 성공이다. 409는 `X-AI-Runtime-Delivery-Status: duplicate`와 동일 event ID를 함께 반환한 경우만 멱등 성공으로 간주한다.
- `Retry-After`는 bounded retry schedule에 반영한다.
- 영구 4xx나 최대 시도 초과는 dead letter로 이동한다.

## 관리 명령

```powershell
go run ./cmd/outboxctl status
go run ./cmd/outboxctl list --state dead
go run ./cmd/outboxctl requeue --id <eventId>
go run ./cmd/outboxctl purge --published-before 720h
```

정확한 옵션은 `-help` 출력을 따른다. dead-letter를 재처리하기 전에 downstream 원인과 payload 호환성을 확인한다.

## reference consumer

`cmd/webhook-sink`는 production money ledger가 아니다. 서명 검증, event idempotency, payload conflict를 로컬·CI에서 확인하는 contract consumer다. 동일 ID·동일 body는 event ID와 duplicate status header를 붙인 409로 멱등 승인하고, 동일 ID·다른 body는 422로 거절한다.

## 남은 money 경계

현재 outbox는 신뢰 가능한 usage event 전달까지 담당한다. 고객의 mandarin 잔액, quote, hold, capture, release, refund의 권위는 별도 money-platform이 가져야 한다. Gateway DB의 usage 값을 고객 잔액처럼 직접 수정하지 않는다.
