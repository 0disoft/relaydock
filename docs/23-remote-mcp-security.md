# 23. Remote MCP Security

## 위협 모델

Remote MCP는 redacted ContextPack과 전문가 결과를 읽고 쓸 수 있으므로 일반 health API보다 강한 권한 경계가 필요하다. 단일 장기 bearer를 Broker 관리 API와 MCP가 공유하면 토큰 유출 하나로 모든 tenant의 상담 결과가 변조된다.

## 자격 증명 종류

```text
Broker bearer
  Expert 관리 HTTP API용

Scoped MCP token
  consultations:read
  consultations:answer
  consultations:admin

Legacy MCP bearer
  로컬 호환용 cluster-wide credential
```

`EXPERT_MCP_TOKEN_SECRET` 또는 `_B64`가 설정되면 scoped HMAC token이 기본이다. 이 상태에서는 `EXPERT_BROKER_BEARER_TOKEN`을 MCP에 자동 재사용하지 않는다. 호환 때문에 꼭 필요할 때만 `EXPERT_MCP_ALLOW_BROKER_TOKEN=true`를 명시한다.

## 토큰 발급

```powershell
go run ./cmd/mcptokenctl issue `
  --subject chatgpt-reviewer `
  --tenant <organizationId> `
  --project <projectId> `
  --scopes consultations:read,consultations:answer `
  --ttl 1h
```

토큰은 audience, token ID, subject, tenant, project, scope, issued-at, expiry를 포함하고 HMAC으로 서명된다. 서버의 `EXPERT_MCP_TOKEN_MAX_LIFETIME`보다 긴 토큰은 검증에서 거절된다. `mcptokenctl`은 알려진 세 scope만 허용하고 read·answer token에는 tenant와 project를 모두 강제한다. `consultations:admin`은 cluster-wide이므로 다른 scope와 혼합 발급하지 않는다. payload와 signature의 비정규 base64url 표현도 거절해 동일 token의 문자열 변형을 막는다.

## tenant 격리

Token ID가 있는 scoped credential은 consultation의 tenant와 project가 모두 일치해야 한다. ContextPack의 tenant/project가 consultation과 다르면 저장 상태 손상으로 처리한다. Consultation ID를 추측해도 다른 project의 pack을 읽거나 결과를 제출할 수 없다.

`consultations:admin`과 legacy static token은 cluster-wide 권한이다. 일반 ChatGPT handoff나 팀 reviewer에게 발급하지 않는다.

## 도구 권한

| MCP tool | 요구 scope |
|---|---|
| `consultation_get` | `consultations:read` |
| `consultation_submit_result` | `consultations:answer` |

결과 제출 token을 읽기 전용 연결에 주지 않는다. Codex bridge는 create/read/cancel 역할을 로컬 IPC로 수행하고, ChatGPT reviewer는 read/answer만 가진다.

## HTTP 통제

- 원격 배포는 TLS 뒤에 둔다.
- `AllowedHosts`로 Host header를 제한한다.
- 브라우저 연결은 `AllowedOrigins`를 명시한다.
- 요청 body 크기를 제한한다.
- loopback 밖에서 인증 없는 MCP를 노출하지 않는다.
- proxy가 원본 Host·Origin을 바꾸는 경우 신뢰하는 reverse proxy 설정을 별도로 검토한다.

## attestation

Remote MCP 제출자는 `modelAttestation`을 제공하지만 provider가 서명한 모델 증명은 아니다. UI와 저장소에는 `remote_mcp` provenance와 submitter를 함께 남긴다. ChatGPT 웹에서 사용자가 선택한 모델은 user-declared 수준이며 공식 API response attestation과 같은 등급으로 표시하지 않는다.

## 회전과 폐기

현재 HMAC secret 회전은 dual-key 자동화가 아니다. rotation 시 새 secret으로 짧은 TTL token을 발급하고, 기존 token 만료를 기다리거나 유지보수 창에서 서버를 전환한다. 장기적으로 OIDC/OAuth와 key ID 기반 다중 검증 키를 추가한다.

## argt2 key ID와 rotation

신규 token은 `argt2.<key-id>.<claims>.<hmac>` 형식을 사용한다. `mcptokenctl` 출력에는 active `keyId`가 함께 들어간다. 서버는 active secret으로만 발급하고 `EXPERT_MCP_TOKEN_VERIFICATION_KEYS_B64`의 retiring secret으로도 검증한다.

```powershell
$env:EXPERT_MCP_TOKEN_KEY_ID = "2026-q3"
$env:EXPERT_MCP_TOKEN_SECRET_B64 = "<new-base64-secret>"
$env:EXPERT_MCP_TOKEN_VERIFICATION_KEYS_B64 = '{"2026-q2":"<old-base64-secret>"}'
```

argt1은 key ID가 없어서 migration 기간에만 모든 verification secret에 대해 확인한다. 마지막 argt1 발급 시점부터 `EXPERT_MCP_TOKEN_MAX_LIFETIME`과 clock-skew가 지난 뒤 `EXPERT_MCP_TOKEN_ALLOW_LEGACY=false`로 전환한다. active key ID를 재사용하면서 secret만 바꾸는 것은 금지한다.
