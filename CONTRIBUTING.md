# Contributing

RelayDock에 의도적으로 제출한 기여는 별도 서면 합의가 없는 한 루트
`LICENSE`의 Apache License 2.0 조건으로 제공된다.

## 브랜치

- `main`: 테스트와 계약 검증을 통과한 코드
- `feat/<area>-<name>`
- `fix/<area>-<name>`
- `adr/<number>-<name>`

## 커밋 전

```powershell
./scripts/check.ps1
```

Linux와 macOS에서는 다음을 사용한다.

```bash
./scripts/check.sh
```

## PR에 반드시 적을 것

- 바뀐 신뢰 경계와 데이터 소유자
- 정상 흐름보다 먼저 검토한 실패 시나리오
- DB·이벤트·공개 API 마이그레이션 여부
- 비밀정보·권한·SSRF·스트림 재시도에 미치는 영향
- 과금과 idempotency에 미치는 영향
- 추가한 자동 테스트와 수동 검증
- 롤백 조건과 방법

## 생성 코드

`buf generate`, `sqlc generate`, `wails3 generate bindings` 결과는 원본 계약과 함께 갱신한다. 생성 파일만 직접 수정한 PR은 받지 않는다.

## 완료 기준

새 기능은 성공 경로 하나로 끝내지 않는다. 취소, timeout, 중복 요청, 부분 스트림, 잘못된 상태 전이, 크기 상한 중 해당되는 실패 경로를 테스트해야 한다. 외부 공급자나 운영 인프라가 없어 실행할 수 없는 코드는 메모리 어댑터나 contract test로 의미를 검증하고, 미검증 범위를 문서에 남긴다.
