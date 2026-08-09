# Database

`db/migrations/`가 운영 데이터베이스의 유일한 변경 이력이다. `cmd/dbmigrate`는 해당 SQL을 바이너리에 embed하고, PostgreSQL advisory lock과 migration별 transaction을 사용해 순서대로 적용한다. 이미 적용된 migration의 이름이나 checksum이 달라지면 실행을 중단한다.

`db/schema.sql`은 모든 migration을 적용한 **현재 최종 스키마 snapshot**이다. sqlc의 타입 검사와 신규 설치 검토에 사용하지만 운영 업그레이드에 직접 실행하지 않는다. 새 migration을 추가할 때는 반드시 다음 세 항목을 같은 변경으로 갱신한다.

1. `db/migrations/<version>_<name>.up.sql`과 `.down.sql`
2. `db/schema.sql`
3. 해당 동작을 검증하는 repository 또는 PostgreSQL integration test

`db/queries/`는 sqlc 생성 계약이다. 핫패스에서 transaction·fencing·동적 쿼리가 필요한 repository는 수기 SQL을 사용할 수 있지만, 두 구현이 동일한 스키마 의미를 공유해야 한다.

CI의 `postgres-integration` 작업은 일회성 PostgreSQL 18 데이터베이스에 embedded migration을 적용한 뒤 runtime request, provider attempt, usage event, transactional outbox, worker lease fencing을 실제로 검증한다. 이 테스트는 테이블을 `TRUNCATE`하므로 전용 폐기 가능한 데이터베이스에서만 실행한다.

Gateway는 money-platform 원장을 소유하지 않는다. 이 데이터베이스에는 `authorization_id`, `price_revision_id`, usage·attempt와 같은 외부 정산 근거만 저장하고, 고객 잔액과 capture 원장은 money-platform이 소유한다.
