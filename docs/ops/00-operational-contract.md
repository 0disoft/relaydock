# Operational Contract Route

- Status: Active index
- Authority: [`../20-operations-runbook.md`](../20-operations-runbook.md)

운영 절차와 복구 의미는 기존 runbook을 직접 갱신한다.

## Routes

- 초기화, key, snapshot, rollout, 장애 복구: [`../20-operations-runbook.md`](../20-operations-runbook.md)
- 개발용 PostgreSQL·Valkey 수직 슬라이스: [`../24-development-stack.md`](../24-development-stack.md)
- release artifact와 배포 gate: [`../15-build-and-release.md`](../15-build-and-release.md)
- 현재 production 미검증 범위: [`../../IMPLEMENTATION_STATUS.md`](../../IMPLEMENTATION_STATUS.md)
- 실제 실행한 검증과 제외 범위: [`../../VALIDATION.md`](../../VALIDATION.md)

명령 예시는 설명일 뿐 현재 워크스페이스의 실행 권한이 아니다. 에이전트 실행 권한은 상위 mustflow command contract가 부여한 intent에 한정된다.
