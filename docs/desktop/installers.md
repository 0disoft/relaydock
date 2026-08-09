# Desktop Installer Route

- Status: Active index
- Current target: Windows-first per-user installation
- Authority: [`../15-build-and-release.md`](../15-build-and-release.md)

현재 installer, signing, sleep/resume, updater rollback은 production 검증이 남아 있다. 구현 완료로 오인하지 않는다.

## Required Sources

- desktop release와 Wails upgrade gate: [`../15-build-and-release.md`](../15-build-and-release.md)
- desktop trust boundary: [`../02-system-context.md`](../02-system-context.md)
- 현재 구현 상태와 미검증 범위: [`../../IMPLEMENTATION_STATUS.md`](../../IMPLEMENTATION_STATUS.md)
- 실제 검증 결과: [`../../VALIDATION.md`](../../VALIDATION.md)

installer 또는 updater 변경은 app·MCP bridge 버전 일치, 서명 실패, 중단 복구, 사용자 데이터 보존, Named Pipe ACL과 rollback을 함께 검증해야 한다.
