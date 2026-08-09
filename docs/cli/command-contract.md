# CLI Contract Route

- Status: Active index
- Authority: command implementation and its owning contract document

`cmd/` 아래 CLI는 조립과 process lifecycle만 소유하며 도메인 규칙을 복제하지 않는다. 전체 실행물 목록과 책임은 [`../03-repository-map.md`](../03-repository-map.md), release 포함 여부는 [`../15-build-and-release.md`](../15-build-and-release.md)를 따른다.

CLI 동작을 변경할 때 함께 확인한다.

- flag, 기본값, stdout/stderr와 exit 의미
- JSON 출력의 하위 호환성과 비밀·파일 본문 노출 여부
- 해당 도메인 contract와 운영 runbook
- 관련 test와 [`../../VALIDATION.md`](../../VALIDATION.md)의 실제 검증 범위

이 문서는 셸 명령 실행 권한을 부여하지 않는다. 에이전트는 상위 mustflow에 등록된 one-shot intent만 실행한다.
