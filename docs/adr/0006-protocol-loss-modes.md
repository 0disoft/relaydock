# ADR-0006: Protocol loss modes

- Status: Accepted
- Date: 2026-08-06
- Owners: Protocol Compiler

## Context

OpenAI Responses, Chat Completions, Anthropic Messages와 Gemini는 reasoning, tool-call delta, continuation, cache metadata 표현이 다르다. 모든 요청을 가장 낮은 공통분모로 평탄화하면 조용한 기능 손실과 디버깅 불가능한 응답이 생긴다.

## Decision

변환 모드는 `strict`, `compatible`, `passthrough` 세 개만 제공한다.

- `strict`: 의미나 공급자 고유 데이터 손실 가능성이 있으면 요청을 거절한다.
- `compatible`: 명시적으로 검증된 변환만 허용하고 loss report를 남긴다.
- `passthrough`: ingress와 upstream이 동일하거나 완전 호환일 때 원문을 우선 보존한다.

silent best-effort 모드는 제공하지 않는다. 미지원 필드를 버리고 성공 응답을 만드는 adapter는 conformance 실패로 처리한다.

## Validation

각 변환 쌍은 golden request·response·stream fixture, unknown event 보존, tool-call fragment, reasoning item, 취소와 부분 EOF 테스트를 가져야 한다. 새 프로토콜 기능은 capability registry와 loss rule을 동시에 추가해야 한다.
