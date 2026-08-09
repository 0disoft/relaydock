# 08. Protocol Compiler

## canonical representation

공통 부분은 canonical item으로 표현한다.

- message
- text
- image
- tool call
- tool result
- reasoning
- refusal
- usage
- provider extension

공급자 고유 필드는 `ProviderExtensions`에 보존한다.

## 변환 모드

### strict

손실 가능성이 하나라도 있으면 요청을 거절한다.

### compatible

사전에 검증된 변환만 허용하고 loss report를 반환한다.

### passthrough

동일 protocol 또는 명시적으로 wire-compatible한 upstream에 원본 의미를 유지한다.

## Loss kind

- field dropped
- field approximated
- ordering changed
- tool identity remapped
- reasoning hidden
- state continuation unavailable
- cache semantics changed
- modality unavailable
- unknown event preserved
- unknown event rejected

## 금지

`messages[]` 하나를 universal internal model로 사용하지 않는다. Responses item, Anthropic block, Gemini part를 억지로 평평하게 만들지 않는다.
