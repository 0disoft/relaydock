# Signing and Token Key Rotation

## Control snapshot Ed25519 rotation

Control snapshot은 `signingKeyId`를 포함한다. Control Plane은 active private key로만 새 revision을 서명하지만, verifier에는 retiring public key를 함께 둘 수 있다. Gateway도 같은 trust ring을 사용하므로 구 snapshot과 신 snapshot이 겹치는 동안 서비스가 끊기지 않는다.

```text
1. 새 Ed25519 keypair 생성
2. Control과 Gateway trust ring에 구키+신키 등록
3. Control active private key와 CONTROL_SIGNING_KEY_ID를 신키로 변경
4. 새 revision publish
5. 모든 Gateway가 신키 snapshot을 적용했는지 확인
6. 최대 LKG·snapshot 수명 경과 후 구 public key 제거
```

관련 환경 변수:

```text
CONTROL_SIGNING_KEY_ID
CONTROL_TRUSTED_SIGNING_PUBLIC_KEYS
CONTROL_ALLOW_LEGACY_SIGNING_KEY_ID

GATEWAY_CONTROL_SIGNING_KEY_ID
GATEWAY_CONTROL_TRUSTED_SIGNING_PUBLIC_KEYS
GATEWAY_CONTROL_ALLOW_LEGACY_SIGNING_KEY_ID
```

추가 trusted key 입력은 JSON 배열 또는 `id=base64,id=base64` 형식을 받는다. key ID는 1~64자의 영문자·숫자·하이픈·밑줄만 허용하고 JSON 뒤에 붙은 두 번째 값도 거절한다. `signingKeyId`가 없던 과거 snapshot은 migration 기간에만 legacy 허용을 유지한다. 모든 LKG와 snapshot이 key ID를 포함한 revision으로 교체된 뒤 양쪽 legacy 스위치를 false로 바꾼다.

Control이 구키로 서명된 persisted snapshot을 읽었지만 trust ring으로 검증할 수 있으면, active key로 다음 revision을 다시 publish한다. 동일 revision의 서명만 바꾸지 않는다. 내용이 같더라도 revision이 증가하므로 Gateway rollback 방어와 충돌하지 않는다.

## Remote MCP HMAC rotation

Remote MCP token v2 형식은 다음과 같다.

```text
argt2.<key-id>.<base64url-claims>.<base64url-hmac>
```

서버는 active key로만 발급하고 `EXPERT_MCP_TOKEN_VERIFICATION_KEYS_B64`에 등록한 retiring key로도 검증한다. JSON 값은 base64 secret이다. normalized key ID 충돌, 64개를 넘는 verification key, 4 KiB를 넘는 secret, 과도한 subject·scope·identity·claims payload는 발급이나 시작 단계에서 거절한다.

```powershell
$env:EXPERT_MCP_TOKEN_KEY_ID = "2026-q3"
$env:EXPERT_MCP_TOKEN_SECRET_B64 = "<new-secret>"
$env:EXPERT_MCP_TOKEN_VERIFICATION_KEYS_B64 = '{"2026-q2":"<old-secret>"}'
```

회전 순서:

```text
1. 새 HMAC secret과 unique key ID 생성
2. 서버 active key를 신키로 변경하고 retiring map에 구키 추가
3. mcptokenctl로 신키 token 발급
4. 기존 token의 최대 수명과 clock-skew가 모두 지난 뒤 구키 제거
5. argt1 token이 모두 만료된 뒤 EXPERT_MCP_TOKEN_ALLOW_LEGACY=false
```

`argt1`에는 key ID가 없으므로 서버가 모든 retiring secret에 대해 HMAC을 확인해야 한다. 이 호환 경로는 영구 기능이 아니다. token 최대 수명이 24시간이라면 마지막 argt1 발급을 중단한 뒤 24시간과 배포 여유시간이 지난 후 끈다.

## 금지 사항

- key ID를 secret으로 취급하지 않는다. 로그에 key ID를 남겨도 되지만 secret은 남기지 않는다.
- active ID에 다른 secret을 덮어쓰지 않는다. 같은 ID의 의미가 바뀌면 incident 분석과 token provenance가 깨진다.
- 구 private key를 Gateway에 배포하지 않는다. Gateway에는 public key만 필요하다.
- rotation 전에 모든 process를 동시에 재시작하는 방식에 의존하지 않는다. overlap trust가 먼저다.
