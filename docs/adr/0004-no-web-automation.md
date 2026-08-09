# ADR-0004: ChatGPT web automation 금지

- Status: Accepted
- Date: 2026-08-06
- Owners: Expert Escalation, Security

## Context

웹 전용 모델을 자동으로 사용하려고 브라우저 DOM, 쿠키, 비공개 endpoint에 의존하면 UI 변경과 계정 정책에 제품 전체가 종속된다. 자격 증명 탈취, 약관 위반, 계정 정지, 응답 출처 위조 위험도 커진다.

## Decision

공식 제품은 DOM scraping, 브라우저 원격조작, 세션 쿠키 추출, 비공개 endpoint 호출을 제공하지 않는다. 웹 모델 경로는 사용자가 ChatGPT에서 상담을 직접 열고 구조화된 결과를 제출하는 handoff로 구현한다. 자동 경로는 공식 API adapter만 사용한다.

## Product contract

웹 handoff 결과의 모델 출처는 `user_declared`로 기록한다. API 결과처럼 공급자 응답으로 검증됐다고 표시하지 않는다. handoff token은 짧은 수명, consultation 단위 scope, 재사용 제한을 가진다.

## Consequences

개인 계정에서는 한 번의 사용자 동작이 남지만, 브라우저 변경으로 서비스가 전면 중단되지 않는다. 향후 공식 Workspace API가 필요한 응답과 모델 attestation을 제공하면 별도 adapter와 ADR로 자동화를 추가할 수 있다.
