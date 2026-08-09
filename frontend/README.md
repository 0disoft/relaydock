# Desktop frontend

Wails v3 웹뷰에 embed되는 Svelte SPA다. 이 앱은 네트워크 API를 직접 호출하지 않고 Wails 생성 binding을 통해 Go 서비스에 접근한다.

## 경계

- 생성 binding import는 `src/lib/runtime-adapter.ts`에만 둔다.
- 공급자 키와 OAuth 토큰을 Svelte store나 localStorage에 저장하지 않는다.
- UI는 장시간 작업 ID를 표시하고 polling 또는 Wails event로 상태를 갱신한다.
- `dist/index.html`은 Go embed가 깨지지 않게 하는 최소 fallback이다. 실제 프론트엔드 빌드가 전체 `dist/`를 교체한다.
