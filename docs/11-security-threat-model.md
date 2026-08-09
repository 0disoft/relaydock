# 11. Security Threat Model

## 주요 자산

- provider credential
- virtual API key
- tenant policy
- ContextPack
- consultation result
- usage event
- money authorization
- update signing key
- local IPC capability

## 공격면

### Local IPC

다른 로컬 사용자가 consultation을 읽거나 tool을 호출할 수 있다.

통제:

- Windows user-scoped Named Pipe ACL
- Unix socket 0600
- nonce와 peer identity
- request size limit
- method allowlist

### Provider URL

사용자 지정 endpoint가 metadata service나 사설망을 공격할 수 있다.

통제:

- scheme allowlist
- DNS resolve 후 IP 검사
- connect 직전 재검사
- redirect마다 재검사
- private range 기본 차단
- port policy

### MCP remote

악성 web origin과 과도한 body가 도구를 호출할 수 있다.

통제:

- OAuth scope
- origin/host validation
- body limit
- rate limit
- read/answer token 분리

### ContextPack

저장소 secret과 고객 데이터가 외부 모델로 전송될 수 있다.

통제:

- local selection
- redaction preview
- explicit approval
- encrypted object
- short TTL
- digest audit

### Updater

악성 artifact로 교체될 수 있다.

통제:

- signed manifest
- artifact hash
- code signing
- rollback
- canary channel
