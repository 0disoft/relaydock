# 06. MCP Contract

## Local STDIO tools

| 도구 | 권한 | 목적 |
|---|---|---|
| `expert_consultation_create` | create | 저장소 맥락을 선별해 상담 생성 |
| `expert_consultation_get` | read | 상태와 전체 구조화 결과 조회 |
| `expert_consultation_cancel` | cancel | 완료 전 상담 취소 |
| `context_pack_preview` | read-local | 전송 후보 파일과 제외 항목 미리보기 |

STDIO MCP는 승인이나 전문가 결과 제출 권한을 갖지 않는다. 사용자가 Wails 화면에서 승인하고, 결과는 API route 또는 별도 ChatGPT connector가 제출한다.

## Remote MCP tools

| 도구 | 권한 | 목적 |
|---|---|---|
| `consultation_get` | read | ChatGPT가 상담과 ContextPack 조회 |
| `consultation_submit_result` | answer | 구조화된 전문가 결과 제출 |

## Token 분리

```text
Codex token
create, read, cancel

ChatGPT connector token
read, answer
```

한 token이 create와 answer를 동시에 갖지 않게 한다. 현재 Remote MCP는 HMAC scoped token으로 `consultations:read`, `consultations:answer`, `consultations:admin`을 구분하고 tenant/project 일치를 검사한다. Legacy static bearer는 cluster-wide 호환 credential이며 scoped token secret이 설정되면 Broker 관리 bearer를 자동 재사용하지 않는다. 장기 운영에서는 OIDC/OAuth와 key ID 기반 rotation으로 교체한다.

## Local IPC

Codex가 시작하는 `cmd/mcp-bridge`는 도메인 로직을 갖지 않는다. STDIO JSON-RPC를 사용자별 Named Pipe 또는 Unix Domain Socket의 길이 제한 JSON frame으로 변환한다. 데스크톱·headless 런타임이 실제 ContextPack과 consultation use case를 수행한다.

## STDIO 규칙

- stdout에는 MCP JSON-RPC만 쓴다.
- 모든 로그는 stderr로 보낸다.
- 시작 banner와 panic stack을 stdout에 출력하지 않는다.
- 장기 모델 호출을 STDIO handler 안에서 기다리지 않는다.
- 요청 body, IPC frame, ContextPack에는 각각 독립적인 크기 상한을 둔다.
- delegation depth가 1을 넘으면 상담 생성 전에 거절한다.
