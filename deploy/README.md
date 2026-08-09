# Deployment

이 디렉터리는 서버 바이너리용 distroless 이미지, migration·bootstrap용 ops 이미지, 전체 개발 Compose, Caddy reverse proxy 예시를 제공한다. Wails 데스크톱 앱은 Docker로 배포하지 않는다.

## 개발 Compose

```powershell
docker compose -f deploy/docker-compose.dev.yml up --build
```

기본 노출:

| 서비스 | 주소 | 개발 인증 |
|---|---|---|
| Gateway | `127.0.0.1:8080` | `development-gateway-token` |
| Control | `127.0.0.1:8081` | `development-control-token` |
| Expert | `127.0.0.1:8082` | `development-expert-token` |
| PostgreSQL | `127.0.0.1:5432` | compose 파일의 개발 계정 |
| Valkey | `127.0.0.1:6379` | 인증 없음 |

이 값은 로컬 전용이다. 외부 호스트나 공유 개발 서버에 그대로 사용하지 않는다.

Compose는 PostgreSQL health 이후 `dbmigrate up`을 단일 실행하고 Gateway를 시작한다. Expert Broker는 기본적으로 writable volume의 local atomic store를 사용한다. PostgreSQL Expert store를 시험할 때는 먼저 `projectctl ensure`로 tenant/project ID를 만든 뒤 `EXPERT_POSTGRES_URL`, `EXPERT_TENANT_ID`, `EXPERT_PROJECT_ID`를 주입한다.

선택적 Caddy proxy:

```powershell
docker compose -f deploy/docker-compose.dev.yml --profile proxy up --build
```

## 이미지

- `Dockerfile.gateway`: stateless Gateway data plane
- `Dockerfile.control`: writable Control snapshot·signing-key 디렉터리 포함
- `Dockerfile.expert`: writable local Expert state 디렉터리 포함
- `Dockerfile.ops`: `dbmigrate`, `projectctl`, `keyctl`

Control과 Expert runtime은 non-root UID로 실행한다. 상태 volume의 ownership을 임의 root UID로 바꾸면 재기동이 실패할 수 있다.

## 초기 운영 기준

- Cloudflare: DNS, WAF, TLS, 정적 자산
- Hetzner: Gateway / Control / Expert
- PostgreSQL: 권위 상태, backup·PITR·restore rehearsal
- Valkey: 재생 가능한 lease·cooldown·rate state만 저장
- R2/S3: 암호화 ContextPack과 짧은 TTL 첨부파일
- migration: 배포당 단일 실행자
- image: immutable digest pin과 SBOM
- secret: Compose 파일이나 image layer에 포함 금지

상세 장애·복구 절차는 `../docs/20-operations-runbook.md`를 본다.
