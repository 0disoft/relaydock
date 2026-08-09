# ADR-0009: MCP Go SDK version

- Status: Accepted
- Date: 2026-08-06
- Owners: MCP

## Context

MCP SDK는 transport, tool schema, progress, cancellation과 authorization 동작을 결정한다. `latest`를 따라가면 Codex 연결이 릴리스마다 달라질 수 있고 생성된 schema가 예고 없이 바뀔 수 있다.

## Decision

Go SDK를 `v1.6.1`에 고정한다. MCP STDIO bridge와 Remote MCP server는 같은 SDK major/minor를 사용한다. SDK 내부 타입을 domain layer에 노출하지 않고 `internal/mcpcontract` DTO를 경계로 둔다.

## Upgrade policy

새 버전은 별도 branch에서 다음을 통과한 뒤 승격한다.

- initialize와 capability negotiation
- tool list와 JSON schema snapshot
- malformed request와 oversized frame
- cancellation·deadline·progress
- stdout 오염 방지
- Codex 실제 연결과 Remote MCP authorization

프로토콜 날짜 지원만 보고 업그레이드하지 않는다. 서버·클라이언트 동작과 기존 도구 이름의 호환성이 우선이다.
