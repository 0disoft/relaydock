# Repository Size and Source Release

## 40 KiB Policy

Keep regular repository files at or below 40 KiB by default. The purpose is to prevent:

- God files that accumulate unrelated responsibilities
- Oversized generated manifests that are difficult to review
- Documents that consume unnecessary prompt and review context
- Accidentally committed build artifacts and binary fixtures

`config/file-size-exceptions.json` permits only files that cannot actually be split, such as upstream fixtures whose byte identity is contractual and canonical lockfiles generated as one file by a package manager.

Source scans retain packaging source in root `build/` but exclude reproducible frontend outputs such as nested `dist/`, nested `build/`, `.svelte-kit/`, `.svelte-check/`, and `node_modules/`.

```json
{
  "version": 1,
  "maxBytes": 40960,
  "exceptions": [
    {
      "path": "tests/fixtures/vendor-binary.dat",
      "reason": "upstream conformance fixture; byte identity is part of the test"
    }
  ]
}
```

Every exception needs a path and concrete reason. Audit fails on stale exceptions, duplicate paths, empty reasons, and nonregular files such as symlinks, sockets, and devices.

```powershell
go run ./cmd/releasepack audit --root .
```

## Chunked Manifest

Split the full file list into a root index and sorted chunks so the manifest itself stays within 40 KiB:

```text
MANIFEST.json
manifest/chunks/files-0001.json
manifest/chunks/files-0002.json
...
```

The root records chunk paths, ranges, file counts, sizes, SHA-256 values, and the aggregate record hash. Chunks target 30 KiB and never exceed 40 KiB. Verification covers contract version, hash algorithm, normalized and nonoverlapping chunk paths, chunk hashes and sizes, record ordering and metadata, aggregate totals, exact repository parity, the 40 KiB policy, unknown fields, and trailing JSON.

## Reproducible Source ZIP

```powershell
go run ./cmd/releasepack build `
  --root . `
  --output ../relaydock-0.5.9-dev-source.zip `
  --generated-at 2026-08-08T12:00:00Z

go run ./cmd/releasepack verify --root .
```

The output must be outside the repository to avoid including an earlier archive in the next archive. Sort ZIP entries by path and normalize every timestamp. Verify mode and size before and after copying, and hash while copying to detect races. Use unique temporary files and publish through a flushed backup swap for Windows rename safety.

`releasepack` generates `TREE.md` and the manifest. `TREE.md` excludes itself and manifest metadata, and the manifest excludes its own hash. These cycle-breaking exclusions are recorded in the root index `excluded` field.
