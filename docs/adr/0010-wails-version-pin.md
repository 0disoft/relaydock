# ADR-0010: Wails version pin

- Status: Accepted
- Date: 2026-08-06
- Owners: Desktop Build

## Context

Wails v3는 빠르게 변경되는 pre-release이며 CLI, Go module, TypeScript runtime과 생성 build assets가 함께 움직인다. CLI만 최신이고 Go module은 이전 버전인 조합은 bindings와 packaging을 깨뜨릴 수 있다.

## Decision

Wails Go module과 CLI를 `v3.0.0-alpha2.119`에 고정한다. CI, bootstrap, Taskfile에서 `@latest` 사용을 금지한다. `@wailsio/runtime`이 추가되면 동일 release 계열로 고정한다.

## Upgrade procedure

1. canary branch에서 Go module, CLI, runtime package를 함께 변경한다.
2. `wails3 update build-assets` 차이를 수동 검토한다.
3. bindings를 재생성하고 불필요한 공개 메서드 노출을 확인한다.
4. Windows·macOS·Linux에서 tray, close-to-tray, `--background`, single-instance를 검증한다.
5. 설치·업데이트·롤백과 MCP bridge 경로 보존을 확인한다.
6. 회귀 결과와 변경 이유를 새 ADR 또는 이 ADR amendment에 남긴다.

검증 전에는 보안 패치처럼 보여도 production pin을 자동 갱신하지 않는다.
