# ADR-0008: License boundary

- Status: Proposed
- Date: 2026-08-06
- Owners: Product, Legal, Open Source

## Context

로컬 runtime과 protocol conformance를 공개하면 adapter 생태계와 신뢰를 얻을 수 있지만, managed control plane·정산·운영 정책까지 동일 조건으로 공개하면 사업 경계가 흐려진다. 아직 최종 법률 검토가 끝나지 않았다.

## Proposed decision

공개 후보는 protocol SDK, local runtime, MCP bridge, provider conformance suite다. 비공개 후보는 managed organization control plane, hosted secret custody, billing integration, abuse controls, 운영 데이터와 내부 deployment automation이다.

## Constraints

- 제3자 코드의 라이선스와 NOTICE 의무를 SBOM에서 추적한다.
- 공개 저장소에 업스트림 소비자 계정 재판매 기능을 넣지 않는다.
- enterprise 디렉터리를 둘 경우 파일별 적용 라이선스를 명확히 한다.
- 최종 LICENSE가 확정되기 전 외부 배포·상업 재사용 가능하다고 홍보하지 않는다.

## Decision gate

변호사 검토, 사업 모델, contributor agreement 필요성, 상표 정책을 결정한 뒤 `LICENSE-PENDING.md`를 실제 LICENSE와 NOTICE로 교체한다. 그 전까지 상태는 Proposed다.
