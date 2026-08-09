# 21. Control Snapshot Distribution

## 목적

Gateway가 요청마다 Control Plane이나 PostgreSQL을 조회하지 않으면서도, 여러 인스턴스가 동일한 모델 route와 가격 revision을 사용하도록 만드는 계약이다. 데이터 플레인은 서명된 불변 snapshot만 소비하고, Control Plane 장애 시 마지막으로 검증한 snapshot으로 제한적으로 계속 동작한다.

## 권위 경계

```text
Control operator
  → controld publish
  → immutable revision store
  → Ed25519 signed snapshot
  → gatewayd verifier
  → atomic in-memory route swap
  → last-known-good file
```

Control Plane만 revision을 발행한다. Gateway는 snapshot을 수정하거나 누락된 candidate를 임의로 보충하지 않는다. snapshot이 적용되면 개발용 implicit `local/echo`를 포함해 문서에 없는 route를 제거한다. PostgreSQL을 사용할 때 `control.runtime_snapshots`는 append-only history이며 가장 높은 committed revision이 현재 상태다.

## 서명 키

`CONTROL_SIGNING_PRIVATE_KEY`는 base64 또는 base64url로 인코딩한 32바이트 Ed25519 seed나 64바이트 private key를 받는다. 값이 없으면 `CONTROL_SIGNING_KEY_PATH`의 per-instance 파일을 사용한다.

여러 Control replica를 사용할 때는 모든 replica가 같은 secret-manager 값을 받아야 한다. replica마다 로컬 키를 만들면 같은 PostgreSQL snapshot store를 공유해도 서명이 뒤섞인다.

공개키는 다음 명령으로 확인한다.

```powershell
go run ./cmd/controlctl signing-key --raw
```

Gateway에는 private key가 아니라 `GATEWAY_CONTROL_SIGNING_PUBLIC_KEY_B64`만 제공한다.

## 발행 규칙

- revision은 반드시 양수이며 이전 revision보다 커야 한다.
- 동일 revision의 재발행과 rollback은 거절한다.
- snapshot은 생성 시각, 만료 시각, 모델 route, provider reference, price revision을 포함한다.
- price revision 목록이 있으면 적용 시각에 이미 유효한 revision이 하나 이상 있어야 하며, 가장 최근 effective revision이 route와 함께 원자 적용된다.
- signature는 canonical unsigned payload에 대해 생성한다.
- PostgreSQL 발행은 serializable transaction과 advisory lock으로 직렬화한다.
- 여러 Control replica가 동시에 초기 revision이나 갱신 revision을 만들면 loser는 최신 committed revision을 다시 읽고 정상 기동한다.

## Gateway 적용 규칙

Gateway는 다음 순서로 시작한다.

1. Control endpoint의 공개키와 설정을 검증한다.
2. 원격 current snapshot을 가져와 signature·시각·revision을 검증한다.
3. 원격 조회가 실패하면 last-known-good 파일을 검증한다.
4. 유효한 snapshot을 route source에 원자 적용한다.
5. watch stream으로 더 높은 revision만 적용한다.
6. 적용이 끝난 뒤에만 last-known-good 파일을 원자 교체한다.

동일 revision인데 내용이 바뀌거나, 더 낮은 revision이 도착하거나, signature가 틀리면 기존 route를 유지하고 새 snapshot을 거절한다. active price revision도 같은 critical section에서 교체하므로 route revision과 usage journal의 price revision이 엇갈리지 않는다.

## 만료와 readiness

`GATEWAY_CONTROL_REQUIRED=true`이면 유효 snapshot이 없을 때 `/readyz`와 신규 모델 요청을 거절한다. 이미 적용된 snapshot도 `expiresAt` 이후에는 사용할 수 없다. Control 장애가 길어지면 LKG가 있어도 만료 시점에서 fail-closed 된다.

만료 시간을 무한히 길게 잡으면 Control이 침해됐거나 route가 폐기돼도 Gateway가 오래된 정책을 계속 사용한다. 운영 기본은 배포·장애 복구 시간을 포함하되 회전 가능한 짧은 TTL로 둔다.

## 로컬 route와의 관계

`GATEWAY_ROUTES_FILE`·`GATEWAY_ROUTES_JSON`은 bootstrap과 단일 노드 개발용이다. 서명 snapshot이 적용되면 전체 route table의 권위가 Control로 넘어간다. `GATEWAY_CONTROL_URL`이 있으면 `provider/model` 직접 호출은 기본적으로 차단되며, `GATEWAY_ALLOW_DIRECT_MODELS=true`를 명시한 경우에만 우회 경로가 열린다. 운영에서는 이 옵션을 끄고 서명된 virtual model만 공개한다.

## 장애 복구

- Control DB 장애: 기존 replica와 Gateway는 마지막 유효 revision을 사용한다.
- signing key 유실: 새 revision을 기존 trust root로 서명할 수 없다. backup에서 복구하거나 명시적 key-rotation 절차가 필요하다.
- LKG 손상: 원격 Control이 정상이라면 재다운로드한다. 원격도 죽었다면 readiness를 내린다.
- 잘못된 publish: 더 높은 수정 revision을 발행한다. 이미 발행한 row를 UPDATE하거나 DELETE하지 않는다.
