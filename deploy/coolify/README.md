# Coolify deployment

초기 관리형 배포는 Gateway, Control, Expert를 별도 서비스로 등록한다. PostgreSQL은 애플리케이션 재배포와 분리하고 외부 백업, PITR, 복구 훈련을 갖춘 인스턴스로 다룬다. Valkey는 재생 가능한 lease·rate-limit 상태만 보유한다.

## Services

| Service | Image | Persistent data | Public exposure |
|---|---|---|---|
| Migration job | `Dockerfile.ops` | 없음 | 비공개 one-shot |
| Control | `Dockerfile.control` | signing key, snapshot state | 관리망 또는 인증된 endpoint |
| Expert | `Dockerfile.expert` | PostgreSQL 사용 권장, local mode면 state volume | 인증된 endpoint |
| Gateway | `Dockerfile.gateway` | 없음 | public API |
| PostgreSQL | managed or dedicated | database volume and backups | private network only |
| Valkey | managed or dedicated | optional AOF, authoritative data 없음 | private network only |

## First deployment

1. 이미지 registry와 immutable digest 정책을 확정한다.
2. PostgreSQL과 Valkey private endpoint를 만든다.
3. secret provider에 provider key, bearer token, virtual-key HMAC key를 등록한다.
4. Control signing key와 snapshot 경로에 persistent volume을 연결한다.
5. `Dockerfile.ops` 이미지로 `dbmigrate up` one-shot job을 실행한다.
6. 같은 ops 이미지의 `projectctl ensure`로 초기 organization과 project를 생성한다.
7. `keyctl issue`로 첫 virtual key를 발급하고 안전한 secret manager에 저장한다.
8. Control과 Expert를 배포하고 `/healthz`, `/readyz`를 확인한다.
9. Gateway를 canary로 배포해 `local/echo`가 아닌 실제 route와 provider를 검증한다.
10. public rollout 뒤 provider usage와 내부 usage event를 대조한다.

## Required deployment checks

- `/healthz`, `/readyz` probe
- migration 단일 실행자
- non-root container의 state directory write permission
- PostgreSQL backup and restore test
- Valkey 장애 시 gateway capacity가 무한 확장되지 않는지 확인
- R2/S3 bucket lifecycle and encryption
- OTLP endpoint와 payload logging 기본값
- 외부 bind 시 authentication enforcement
- Caddy 또는 upstream proxy의 request body·idle timeout이 streaming 요구와 일치

## Upgrade

```text
backup checkpoint
  → ops/dbmigrate up
  → control
  → expert
  → gateway canary
  → gateway rollout
```

Coolify의 자동 재배포가 migration job을 여러 인스턴스에서 동시에 실행하지 않도록 한다. Migration runner 자체도 PostgreSQL advisory lock을 사용하지만 배포 오케스트레이션에서 단일 실행을 유지한다.
