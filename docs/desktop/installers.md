# Desktop Installer Route

- Status: Active index
- Current target: Windows-first per-user installation
- Authority: [`../15-build-and-release.md`](../15-build-and-release.md)

Production validation is still pending for installers, signing, sleep/resume, and updater rollback. Do not mistake these surfaces for completed production work.

## Required Sources

- Desktop release and Wails upgrade gates: [`../15-build-and-release.md`](../15-build-and-release.md)
- Desktop trust boundary: [`../02-system-context.md`](../02-system-context.md)
- Current implementation and unverified scope: [`../../IMPLEMENTATION_STATUS.md`](../../IMPLEMENTATION_STATUS.md)
- Actual validation results: [`../../VALIDATION.md`](../../VALIDATION.md)

Installer or updater changes must verify app/MCP bridge version parity, signature failures, interrupted recovery, user-data preservation, Named Pipe ACLs, and rollback together.
