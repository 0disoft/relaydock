# ADR-0001: Go-first runtime

- Status: Accepted
- Date: 2026-08-06
- Owners: Runtime, Desktop, Control Plane

## Context

제품의 핵심은 장시간 연결, SSE·NDJSON 스트림, 취소 전파, 공급자별 HTTP 연결 풀, 로컬 데몬, MCP STDIO와 운영 서버다. UI 편의를 이유로 Node와 Rust를 추가하면 동일한 프로토콜·상태 머신·보안 규칙을 여러 언어에서 중복 구현하게 된다.

## Decision

Gateway, Control Plane, Expert Broker, headless runtime, MCP bridge와 Wails 데스크톱 백엔드를 Go로 구현한다. TypeScript는 Svelte UI, 생성된 API 타입, 브라우저 입력 검증에만 사용한다. Python은 운영 요청 경로에 넣지 않고 평가 fixture와 오프라인 분석에만 허용한다.

핵심 도메인 패키지는 Wails, HTTP 프레임워크, 데이터베이스 드라이버를 import하지 않는다. 외부 기술은 `internal/desktopwails`, `internal/persistence`, `internal/transport` 어댑터 안에 격리한다.

## Consequences

- 프로토콜 컴파일러와 ContextPack 로직을 desktop·server·headless가 공유한다.
- 단일 Go toolchain과 build cache를 사용한다.
- UI 전용 로직을 Go 서비스로 밀어 넣지 않아야 한다.
- Go에서 불편하다는 이유만으로 공급자별 스크립트 런타임을 추가하지 않는다.

## Validation

`go test -race ./internal/... ./tests/...`, 스트림 golden test, MCP bridge 통합 테스트가 통과해야 한다. 새 런타임 언어 도입은 별도 ADR과 배포·관측·보안 비용 분석 없이는 허용하지 않는다.
