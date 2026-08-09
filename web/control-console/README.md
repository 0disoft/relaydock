# Control Console

관리형 Control Plane을 조회하는 SvelteKit 서버 앱이다. 브라우저가 제어면 bearer token을 직접 받지 않도록 서버 `load` 함수가 Control API의 상태와 모델 경로를 읽는다.

## 환경 변수

```text
CONTROL_API_BASE_URL=http://127.0.0.1:8081
CONTROL_BEARER_TOKEN=
```

현재 화면은 상태, 스냅샷 리비전, 만료 시각, 가상 모델 경로를 읽기 전용으로 제공한다. 조직·프로젝트·가상 키 변경은 인증과 감사 로그 계약이 연결된 뒤 별도 route action으로 추가한다.

## 실행

```bash
bun install
bun run dev
```

공급자 비밀 원문은 브라우저로 전달하지 않는다. 결제·잔액 원장은 이 앱이 소유하지 않고 money-platform으로 위임한다.
