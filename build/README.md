# Build and Packaging

`task build` creates the Wails desktop executable and `mcp-bridge` for the current operating system. `task package` collects both executables, the README, Apache-2.0 `LICENSE`, and `NOTICE` into a portable ZIP or `tar.gz` bundle.

```powershell
wails3 doctor
wails3 build
task package
```

Portable bundles are intended for development and internal deployment validation. Refresh native installation assets such as Windows NSIS/MSIX, macOS `.app` and DMG, and Linux AppImage/DEB/RPM with the generator from the pinned Wails version.

```powershell
wails3 update build-assets
wails3 package
```

When merging generated assets, verify all of the following:

- Installation is per-user by default.
- The desktop app and `mcp-bridge` are installed in the same directory.
- Autostart launches the desktop executable with `--background`.
- Sign the Windows executable and installer separately.
- Verify the macOS hardened runtime, entitlements, and notarization in the release pipeline.
- Keep update-manifest and artifact-signing keys separate from code-signing keys.
- Preserve user settings and MCP paths after installation, update, and rollback.

`build/config.yml` is the source of truth for product metadata. Do not edit product names or identifiers separately in generated platform files.
