# AGENTS.md

## 저장소 목적

이 저장소는 일반 챗봇이 아니라 AI 요청의 프로토콜·라우팅·비용·상담 handoff를 통제하는 런타임이다. 기능 개수보다 경계와 실패 의미가 중요하다.

## 반드시 지킬 규칙

### Wails

- `internal/desktopwails`만 Wails package를 import한다.
- Wails service는 DTO 변환과 use-case 호출만 담당한다.
- 프론트엔드 binding 호출은 얇은 adapter 뒤에 둔다.
- 창 종료와 런타임 종료를 동일시하지 않는다.
- 두 번째 인스턴스의 args와 additional data는 불신 입력으로 취급한다.

### MCP

- STDIO transport에서 stdout에는 MCP message 외의 것을 절대 쓰지 않는다.
- 모든 로그는 stderr로 보낸다.
- tool handler에서 장기 작업 완료를 기다리지 않고 consultation ID를 반환한다.
- Codex token에는 create/read/cancel만, ChatGPT connector token에는 read/answer만 부여한다.
- delegation depth를 1보다 크게 만들지 않는다.

### 프로토콜 변환

- 모르는 필드와 event를 즉시 삭제하지 않는다.
- strict mode에서 손실이 하나라도 있으면 거절한다.
- compatible mode의 모든 변환은 loss report에 남긴다.
- passthrough는 upstream과 ingress가 같은 의미 계약일 때만 허용한다.
- 첫 semantic event 이후 자동 retry는 금지한다.

### 과금

- PostgreSQL이 권위 상태다.
- Valkey는 lease, rate limit, cooldown, affinity에만 쓴다.
- usage event와 customer charge를 같은 row로 뭉치지 않는다.
- price revision은 요청 시작 시 고정한다.
- 조정은 원본 row 수정이 아니라 adjustment entry로 기록한다.
- idempotency key 없는 capture API를 만들지 않는다.

### 보안

- 임의 provider URL과 MCP URL은 SSRF 검사 후 연결한다.
- context pack은 local에서 선별·redact한 뒤 전송한다.
- prompt/response 본문 logging은 기본 비활성이다.
- ChatGPT 웹 DOM 자동화, 쿠키 추출, 비공개 endpoint 흉내를 공식 기능으로 넣지 않는다.
- secret 값은 error와 structured log에 들어가면 안 된다.

### 코드

- 생성 디렉터리는 직접 수정하지 않는다.
- TODO를 성공 처리로 위장하지 않는다.
- 외부 의존성이 필요한 경로는 명시적 설정 오류를 반환하고, 로컬 메모리 구현으로 수직 흐름을 검증한다.
- public interface 변경에는 ADR 또는 contract 문서 변경이 따라야 한다.
- 상태 전이는 switch 문 곳곳이 아니라 한 state machine에 모은다.
- provider별 예외를 canonical core에 직접 박지 않는다.

## 생성 디렉터리

```text
frontend/bindings/
gen/go/
internal/persistence/postgres/sqlcgen/
```

## 테스트 완료 조건

변경 영역에 따라 최소 하나 이상을 추가한다.

- golden wire fixture
- state transition test
- fuzz test
- fault injection test
- billing idempotency test
- Playwright UI test
- migration round-trip test

## 리뷰 우선순위

1. 이중 차감·권한 상승·비밀 유출
2. 중간 스트림 중복 실행
3. 손실 있는 프로토콜 변환
4. 상태 머신 우회
5. 취소·timeout 누락
6. 무제한 메모리·본문·동시성
7. 관측 불가능한 실패
8. 유지보수성
