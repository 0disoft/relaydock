# 07. ContextPack

## 포함 내용

- objective
- success criteria
- repository revision
- working tree digest
- architecture summary
- 관련 파일 조각
- 관련 테스트
- 오류 로그
- 이미 실패한 접근
- 금지된 대안
- open questions

## 파일 선택

Selector는 다음 신호를 조합한다.

- 명시적으로 언급된 파일
- failing test import graph
- git diff
- symbol reference
- runtime stack trace
- architecture ownership map
- 사용자 pin

## Redaction

전송 전 local에서 실행한다.

- 경로 denylist
- `.env`, key, certificate 기본 제외
- known token prefix
- high entropy
- connection string
- 개인정보 pattern
- 사용자 custom rule

## 무결성

모든 evidence에 digest와 line range를 남긴다. 상담 결과를 받을 때 현재 파일 digest와 비교해 stale 여부를 표시한다.

## 상한

- 최대 파일 수
- 최대 byte
- 최대 추정 token
- 바이너리 제외
- generated code 기본 제외
- dependency vendor 기본 제외
