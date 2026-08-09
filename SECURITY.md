# Security Policy

## 비공개 제보 대상

- virtual key 우회
- tenant 경계 우회
- provider credential 노출
- ContextPack secret 유출
- local IPC 권한 우회
- DNS rebinding / SSRF
- usage 위조 또는 이중 capture
- MCP tool scope 우회
- updater signature 우회

공개 issue에 token, cookie, prompt 원문, 고객 데이터, exploit payload를 올리지 않는다.

## 기본 보안 태도

- fail closed: 인증, 결제 capture, secret redaction, 원격 MCP write
- fail open 금지: tenant scope, provider credential, usage ledger
- 마지막 유효 snapshot 사용 가능: 단 snapshot 만료 정책을 tenant가 명시한 경우
