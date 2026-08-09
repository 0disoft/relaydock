# Project Invariants Route

- Status: Active index
- Binding rules: [`../../AGENTS.md`](../../AGENTS.md)

구현 중 반드시 보존할 invariant는 `AGENTS.md`가 정의한다. 특히 다음 경계는 우회하지 않는다.

- 첫 semantic event 이후 자동 provider retry 금지
- strict protocol 변환에서 손실 발생 시 거절
- PostgreSQL만 과금 권위 상태를 소유
- STDIO stdout에는 MCP message만 기록
- 비밀과 prompt/response 본문을 기본 로그에 기록하지 않음
- Wails import를 `internal/desktopwails` 밖으로 확산하지 않음

상세 구조는 [`../03-repository-map.md`](../03-repository-map.md), 검증 요구는 [`../../VALIDATION.md`](../../VALIDATION.md), 작업별 점검은 [`../../CHECKLIST.md`](../../CHECKLIST.md)를 따른다. 이 문서는 별도 규칙의 권위가 아니다.
