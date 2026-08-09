# ADR-0007: Money platform boundary

- Status: Accepted
- Date: 2026-08-06
- Owners: Accounting, Money Platform

## Context

Gateway는 provider token usage와 attempt를 가장 잘 알지만 고객 잔액, 충전, 환불, 만료, 세금과 결제 원장을 소유하면 안 된다. 모델 공급자를 교체할 수 있는 data plane과 금전 원장을 한 DB에 묶으면 장애와 마이그레이션 영향이 확대된다.

## Decision

Gateway는 request, provider attempt, measured usage, price revision reference를 소유한다. money-platform은 balance, authorization hold, capture, release, adjustment, refund를 소유한다. Gateway DB에 고객 잔액을 복제하지 않는다.

정산 계약은 `quote → hold → provisional usage → capture/release → adjustment` 순서를 사용한다. 모든 변경 명령은 idempotency key를 요구하고 원본 원장 행을 수정하지 않는다.

## Failure policy

money-platform이 불가용하면 상품 정책에 따라 신규 유료 요청을 fail-closed하거나 제한된 사전 승인 lease만 사용한다. Valkey 잔액을 진실로 사용하거나 실패한 capture를 성공으로 표시하지 않는다.

## Validation

중복 delivery, timeout 후 재시도, provider usage 수정, 부분 스트림 비용, 환불과 reconciliation contract test를 통과해야 한다.
