# Build and packaging

`task build`는 현재 운영체제용 Wails 데스크톱 실행 파일과 `mcp-bridge`를 함께 만든다. `task package`는 두 실행 파일, README, 라이선스 상태 문서를 한 디렉터리에 모아 ZIP 또는 `tar.gz` 휴대용 번들로 만든다.

```powershell
wails3 doctor
wails3 build
task package
```

휴대용 번들은 개발·사내 배포 검증용이다. Windows NSIS/MSIX, macOS `.app`·DMG, Linux AppImage/DEB/RPM 같은 네이티브 설치 자산은 Wails 고정 버전의 생성기를 통해 갱신한다.

```powershell
wails3 update build-assets
wails3 package
```

생성된 자산을 병합할 때 다음 조건을 반드시 검토한다.

- 설치 범위는 기본적으로 per-user다.
- 데스크톱 앱과 `mcp-bridge`가 같은 설치 디렉터리에 배치돼야 한다.
- 자동 실행 등록은 데스크톱 실행 파일에 `--background`를 전달해야 한다.
- Windows 실행 파일과 설치 프로그램은 각각 서명한다.
- macOS hardened runtime, entitlements, notarization을 릴리스 파이프라인에서 검증한다.
- 업데이트 manifest와 artifact 서명 키를 코드 서명 키와 분리한다.
- 설치·업데이트·롤백 후 사용자 설정과 MCP 경로가 유지되는지 검사한다.

`build/config.yml`이 제품 메타데이터의 기준이고, 생성된 플랫폼 파일에서 제품명이나 식별자를 따로 수정하지 않는다.
