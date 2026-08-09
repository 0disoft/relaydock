# ADR-0003: Local IPC

- Status: Accepted
- Date: 2026-08-06
- Owners: Desktop Runtime, MCP

## Context

Codex는 MCP STDIO 프로세스를 세션마다 시작하지만, 공급자 상태와 상담 기록은 사용자별 장기 실행 런타임에서 공유해야 한다. localhost TCP는 포트 충돌, 방화벽 노출, DNS rebinding과 불필요한 인증 표면을 만든다.

## Decision

MCP bridge와 desktop/headless runtime은 Windows Named Pipe 또는 Unix Domain Socket으로 통신한다. 메시지는 길이 접두사가 있는 제한된 JSON frame을 사용한다. bridge는 STDIO JSON-RPC와 내부 IPC를 변환할 뿐 도메인 로직과 자격 증명을 보유하지 않는다.

## Security rules

- Unix socket은 사용자 전용 디렉터리와 `0600` 권한을 사용한다.
- Windows pipe는 현재 사용자 ACL 검증을 릴리스 조건으로 둔다.
- frame 최대 크기를 초과하면 연결을 끊고 요청을 실행하지 않는다.
- stdout은 MCP 프로토콜 전용이며 로그는 stderr로만 보낸다.
- endpoint 이름에 tenant·token·repository 경로를 넣지 않는다.

## Failure handling

runtime이 없으면 bridge는 명확한 연결 오류를 반환한다. 요청을 성공 처리하거나 임시 독립 runtime을 몰래 만들지 않는다. 부분 frame, EOF, deadline, 취소는 각각 오류로 분류한다.
