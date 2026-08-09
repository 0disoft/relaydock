# 00. 제품 정체성

## 한 줄 정의

코딩 에이전트가 평소에는 빠른 모델로 작업하고, 고난도 설계·보안·과금·마이그레이션 판단이 필요할 때 더 강한 전문가 경로로 **검증 가능한 맥락만** 넘긴 뒤 결과를 다시 구현 작업으로 가져오는 AI Runtime Gateway다.

## 경쟁 제품과 다른 기준

이 제품은 “모델 수가 많은 프록시”를 목표로 하지 않는다.

핵심 자산은 다음 다섯 개다.

1. 공급자별 reasoning·tool·stream 의미를 보존하는 protocol compiler
2. 첫 토큰 전후를 구분하는 retry semantics
3. 코드·테스트·실패 이력을 묶는 ContextPack compiler
4. 전문가 결과를 구현과 검증 조건으로 되돌리는 consultation contract
5. 공급자 usage와 고객 charge를 분리하는 정산 구조

## 대상 사용자

- Codex·Claude Code·OpenCode를 오래 쓰는 개인 개발자
- 여러 provider와 자체 모델을 섞는 소규모 팀
- 모델 비용과 권한을 중앙 통제해야 하는 제품 조직
- AI API를 자신의 제품에 제공하는 인디해커

## 제품 면

하나의 브랜드 아래 세 가지 제품 면을 둔다.

| 면 | 설치 위치 | 핵심 가치 |
|---|---|---|
| Desktop Runtime | 사용자 PC | 로컬 키·MCP·ContextPack·전문가 handoff |
| Managed Gateway | 서버 | virtual key·protocol·routing·usage |
| Control Console | 웹 | 조직·정책·비용·감사 |

이 셋은 UI에서 연결되지만 신뢰 경계와 배포물은 분리한다.

## 성공 판정

MVP 성공은 provider 숫자로 판단하지 않는다.

- 같은 입력이 provider 변환 중 어떤 의미를 잃는지 설명 가능
- 중간 스트림 실패를 성공으로 기록하지 않음
- 전문가에게 보낸 파일과 revision을 재현 가능
- 동일 usage가 두 번 capture되지 않음
- 데스크톱 agent가 Codex 재실행 없이 안정적으로 붙음
