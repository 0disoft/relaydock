# 17. Open Questions

이미 코드로 결정된 항목은 이 문서에서 제거한다. 아래 질문은 구현 전에 ADR이나 운영 정책 revision으로 닫아야 한다.

## 제품과 패키징

- 별도 비공개 managed service 저장소와 공개 RelayDock API의 운영 경계
- enterprise self-host 지원·보증·상표 사용의 상용 계약
- 개인 무료판의 provider·route·Expert 호출 상한

## Wails 배포

- Windows 기본 설치 형식: NSIS 또는 MSIX
- updater의 실제 process replacement와 rollback 책임
- macOS notarization·entitlement 범위
- Linux 배포 대상과 WebKitGTK 최소 기준
- Wails v3 alpha pin을 해제할 release gate

## MCP와 Expert

- Remote MCP의 OAuth/OIDC issuer와 조직 RBAC 모델
- ChatGPT 개인 플랜용 result import UX
- user-declared model attestation의 UI 표현
- API Expert 실패 비용을 고객에게 넘길지 플랫폼이 부담할지
- 복수 Expert panel의 최대 delegation·비용 정책

## ContextPack

- symbol index를 Go parser 중심으로 만들지 tree-sitter를 붙일지
- gitignored 파일의 기본 제외와 수동 포함 정책
- 공급자별 tokenizer를 넣을 시점
- binary artifact·이미지·대형 로그의 요약 adapter
- R2/S3 upload의 encryption key 소유권과 TTL

## Gateway와 routing

- production에서 signed Control snapshot을 유일 SSOT로 강제할 전환 시점
- process-local cooldown을 Valkey로 공유할 데이터 모델
- provider health probe 주기와 false positive 억제 방식
- session affinity와 prompt-cache affinity key의 개인정보 경계
- same-provider stream resume를 허용할 공급자 목록

## Accounting

- 실패 attempt 비용의 customer charge 정책
- provider usage 수정 도착을 기다릴 최대 기간
- 환율과 mandarin price revision 시점
- 무료·유료·프로모션 크레딧 소진 순서
- 조정 원장의 최소 보존 기간과 감사 export 형식

## Infra와 운영

- 초기 단일 PostgreSQL의 schema-level ACL
- managed object store 기본값: R2, B2, S3
- provider별 egress region과 data residency
- PostgreSQL polling outbox의 처리량 상한과 broker 도입 trigger
- single-region 복구 목표와 multi-region 진입 조건
