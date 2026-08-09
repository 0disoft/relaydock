# Product Specification Route

- Status: Active index
- Authority: existing numbered contracts

RelayDock의 단일 대형 제품 명세는 두지 않는다. 변경 성격에 맞는 계약을 읽고 그 문서를 직접 갱신한다.

| 변경 영역 | 권위 문서 |
|---|---|
| 제품 목적·사용자·성공 기준 | [`../00-product-identity.md`](../00-product-identity.md) |
| 단계별 범위·비목표 | [`../01-scope-and-non-goals.md`](../01-scope-and-non-goals.md) |
| 시스템과 신뢰 경계 | [`../02-system-context.md`](../02-system-context.md) |
| 요청·retry·취소 의미 | [`../04-request-lifecycle.md`](../04-request-lifecycle.md) |
| MCP·ContextPack·protocol·routing·accounting | [`../06-mcp-contract.md`](../06-mcp-contract.md), [`../07-context-pack.md`](../07-context-pack.md), [`../08-protocol-compiler.md`](../08-protocol-compiler.md), [`../09-routing.md`](../09-routing.md), [`../10-accounting.md`](../10-accounting.md) |
| 보안과 위협 모델 | [`../11-security-threat-model.md`](../11-security-threat-model.md) |
| 구현 상태·검증 결과 | [`../../IMPLEMENTATION_STATUS.md`](../../IMPLEMENTATION_STATUS.md), [`../../VALIDATION.md`](../../VALIDATION.md) |

공개 인터페이스 변경에는 `AGENTS.md` 규칙에 따라 해당 contract 문서 또는 ADR 변경이 따라야 한다.
