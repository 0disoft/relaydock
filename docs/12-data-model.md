# 12. Data Model

## Schema ownership

| schema | 소유 모듈 |
|---|---|
| `control` | controld와 bootstrap CLI |
| `runtime` | gatewayd journal |
| `expert` | expert-brokerd |
| `outbox` | transaction producer와 outboxd |

Migration 파일이 schema 변경 이력의 SSOT이고 `db/schema.sql`은 모든 migration 적용 뒤의 검증 snapshot이다.

## control

- `organizations`
- `projects`
- `virtual_keys`
- `provider_connections`
- `model_routes`
- `runtime_snapshots`

`runtime_snapshots`는 revision append-only다. Payload와 signature, generated/expiry columns가 일치하지 않으면 손상으로 처리한다.

## runtime

### requests

사용자 관점의 한 요청이다.

- tenant/project/virtual-key identity
- client request ID
- ingress protocol과 virtual model
- price revision과 authorization reference
- state와 first semantic commit 시각
- attempt count와 모든 attempt의 token 합계
- terminal error와 완료 시각

### provider_attempts

Retry·fallback마다 별도 row다.

- request ID와 attempt number
- provider, account, upstream model, protocol
- committed 여부
- attempt usage와 terminal error
- 시작·완료 시각

### usage_events

Provider report와 local estimate를 보존하는 상세 usage row다. Request·attempt summary와 함께 reconciliation 근거로 사용한다.

## expert

- `context_packs`
- `consultations`
- `consultation_results`

Consultation은 tenant/project scope, idempotency fingerprint, queue availability, worker lease, attempts, failure reason, result reference를 가진다. Result commit은 worker fencing을 통과해야 한다.

## outbox

`events`는 aggregate event의 delivery state를 가진다.

- topic, aggregate ID, JSON payload
- available time
- locked worker와 lease expiry
- attempt count와 last error
- published 또는 dead-letter time

Request terminal update와 event insert가 같은 transaction에서 수행된다. Consumer는 event ID를 idempotency key로 사용한다.

## 경계

다른 schema table을 임의 update하지 않는다. Cross-domain 전달은 API, immutable snapshot, transactional outbox를 사용한다. Valkey state는 이 schema의 권위 데이터를 대체하지 않는다.
