# ADR-0002: Wails v3 desktop

- Status: Accepted with risk
- Date: 2026-08-06
- Owners: Desktop Runtime

## Context

데스크톱 앱은 Go 로컬 런타임, 시스템 트레이, 단일 인스턴스, 자동 시작, 네이티브 창과 Svelte UI를 묶어야 한다. Tauri를 채택하면 이미 Go로 구현한 런타임 위에 Rust shell과 Cargo build graph가 추가된다.

## Decision

Tauri를 사용하지 않고 Wails v3를 사용한다. Wails import는 `internal/desktopwails`와 루트 실행 진입점에만 허용한다. Wails 서비스에는 얇은 use-case 호출만 두고 protocol, routing, accounting 로직을 넣지 않는다.

데스크톱 프로세스는 per-user tray agent다. 로그인 자동 실행은 `--background` 인수로 창을 숨긴 채 IPC를 시작한다. GUI가 필요 없는 환경은 별도 `cmd/headless`를 사용한다.

## Risks and controls

- v3 pre-release API 변경 위험은 exact pin과 canary upgrade로 제한한다.
- 창 종료, sleep/wake, WebView2 재생성, single-instance, updater는 플랫폼 실기기에서 검증한다.
- 바인딩 생성이 실패하지 않도록 공개 Wails service 메서드에는 직렬화 불가능한 인터페이스나 `context.Context` 인수를 노출하지 않는다.
- Wails 자체 상태를 도메인 SSOT로 사용하지 않는다.

## Exit strategy

Wails 교체가 필요하면 `internal/desktopwails`와 build assets만 대체한다. 로컬 IPC, ContextPack, provider routing, consultation 저장소 계약은 유지한다.
