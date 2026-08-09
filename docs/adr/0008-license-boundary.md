# ADR-0008: License boundary

- Status: Accepted
- Date: 2026-08-09
- Owners: Product, Legal, Open Source

## Context

RelayDock는 sub2api·OpenCodex 계열의 대체 가능한 공개 AI runtime을 목표로 한다. 같은 저장소 안에서 일부 경로만 비공개로 선언하면 기여자와 배포물의 권리 경계가 불명확해지고, source archive와 container가 실제로 어떤 조건을 따르는지 자동 검증하기도 어렵다.

## Decision

이 저장소의 source, 문서, 생성 계약과 RelayDock가 배포하는 산출물 전체를 Apache License 2.0으로 공개한다. 루트 `LICENSE`가 유일한 저장소 라이선스이며 package metadata도 `Apache-2.0`으로 맞춘다.

향후 0disoft가 hosted secret custody, 비공개 abuse policy, 내부 운영 데이터나 독점 deployment automation을 개발할 경우 이 저장소에 `enterprise/` 같은 예외 디렉터리를 만들지 않고 별도 비공개 저장소에서 운영한다. 공개 RelayDock와의 접점은 versioned public API·protocol 계약으로만 둔다.

## Constraints

- 제3자 코드의 라이선스와 NOTICE 의무를 SBOM에서 추적한다.
- 공개 저장소에 업스트림 소비자 계정 재판매 기능을 넣지 않는다.
- dependency 또는 외부 자료를 도입할 때 Apache-2.0과의 호환성과 attribution 의무를 검토한다.
- `NOTICE`는 프로젝트 attribution을 보존하며, 제3자 필수 고지는 release SBOM 검토 결과에 따라 갱신한다.

## Consequences

- source와 binary를 같은 조건으로 배포할 수 있고 contributor의 기본 제출 조건도 Apache-2.0 제5조를 따른다.
- 상표 사용권은 Apache-2.0이 부여하지 않으므로 프로젝트 이름과 로고 정책은 별도로 정할 수 있다.
- CLA·DCO나 별도 상표 정책은 나중에 추가할 수 있지만 현재 공개 배포를 막는 라이선스 미결정 상태는 해소한다.
