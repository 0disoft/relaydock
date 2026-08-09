# Tests

테스트 디렉터리는 제품 경계에 맞춰 나눈다.

- `conformance`: ingress/egress 프로토콜 계약
- `golden_streams`: 공급자 이벤트 재생과 순서 검증
- `fault_injection`: 중간 연결 종료, 취소, 429/5xx, backpressure
- `billing`: usage idempotency와 quote-hold-capture 정산
- `expert`: ContextPack, redaction, 상담 상태 머신, 재귀 호출 방지
- `load`: TTFT, inter-token latency, queue delay를 분리 측정

실제 공급자 live test는 기본 CI에서 실행하지 않는다. 별도 nightly probe와 제한된 자격 증명을 사용한다.
