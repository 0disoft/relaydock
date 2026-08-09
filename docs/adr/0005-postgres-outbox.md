# ADR-0005: PostgreSQL transactional outbox

- Status: Accepted
- Date: 2026-08-06
- Owners: Persistence, Expert Broker

## Context

상담 상태 변경, usage 확정, 정산 요청은 데이터 변경과 이벤트 발행이 함께 성공해야 한다. 초기 규모에서 Kafka·NATS를 먼저 도입하면 dual-write 문제는 남고 운영 장애 지점만 늘어난다.

## Decision

도메인 행 변경과 outbox insert를 같은 PostgreSQL transaction에 기록한다. worker는 `FOR UPDATE SKIP LOCKED` 기반 atomic claim, worker ID, lease 만료 시각을 사용한다. 처리 성공 후 publish 시각을 기록하고, 실패하면 backoff와 다음 실행 시각을 갱신한다.

## Invariants

- outbox payload에는 secret과 원문 prompt를 넣지 않는다.
- event ID는 전역적으로 유일하며 소비자는 idempotent해야 한다.
- claim과 lease 갱신은 한 statement 또는 한 transaction에서 수행한다.
- 행 삭제는 보존 정책과 downstream reconciliation 완료 후에만 허용한다.

## Scale trigger

DB CPU, lock wait, outbox 지연, 보존량이 합의된 SLO를 넘을 때만 broker를 도입한다. 도입 후에도 PostgreSQL outbox는 transaction boundary로 남고 relay가 broker에 전달한다.
