# 05. Expert Escalation

## 역할 분리

```text
Expert
판단, 비판, 대안, 검증 조건

Coding Agent
파일 수정, 명령 실행, 테스트, 결과 대조
```

Expert route는 저장소 쓰기 권한을 기본으로 가지지 않는다.

## Route

### `openai_api_pro`

공식 API를 자동 호출한다. 실행 모델과 usage를 provider 응답으로 확인할 수 있다.

### `chatgpt_web_handoff`

Codex가 consultation과 ContextPack을 만들고 사용자가 ChatGPT에서 직접 검토한다. 브라우저 DOM을 자동조작하지 않는다.

### `workspace_agent`

기업 환경에서 검증 후 추가할 확장 route다. 기본 MVP에서 제외한다.

## 자동 승격 조건

- 결제·인증·권한
- irreversible schema migration
- public API breaking change
- 서로 다른 수정 두 번 실패
- agent 분석 충돌
- 사용자 명시 호출

## 비용 폭주 방지

- task당 최대 호출 수
- task당 최대 예상 비용
- delegation depth 1
- 같은 ContextPack digest에 대한 중복 실행 방지
- approval policy
