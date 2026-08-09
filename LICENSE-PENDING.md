# License decision pending

이 저장소에는 아직 최종 배포 라이선스를 부여하지 않았다. 코드는 제품 구현과 내부 검증을 위한 개발 소스이며, 공개 배포 전에 라이선스 경계를 확정해야 한다.

현재 제안은 다음과 같다.

- protocol SDK, `gatewayd`, MCP bridge, desktop runtime: Apache-2.0 후보
- managed Control Plane, hosted accounting integration: 비공개 또는 상용 라이선스 후보
- 공급자별 상표·SDK·샘플 데이터: 각 원저작권과 약관을 별도로 확인

공개 저장소로 push하거나 바이너리를 배포하기 전에 `docs/adr/0008-license-boundary.md`를 승인하고 루트 `LICENSE`와 제3자 고지 파일을 추가해야 한다.
