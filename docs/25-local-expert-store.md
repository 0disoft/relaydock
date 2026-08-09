# Local Expert Store

## Purpose

Desktop and headless executions must retain consultations and ContextPacks without PostgreSQL, while avoiding a rewrite of one large JSON file whenever consultation state changes. Local store v3 separates a **small atomic metadata file** from **immutable content-addressed chunks**.

## Layout

```text
expert-state.json
expert-state.json.chunks/
+-- ab/
    +-- cdef...        # full digest is sha256:abcdef...
```

Metadata contains consultations, idempotency records, ContextPack manifests, results, and chunk references for source content. Split source into chunks of at most 32 KiB and address them by SHA-256. Multiple ContextPacks reuse identical content.

## Write Contract

Write immutable ContextPack chunks first, then atomically replace metadata with the ContextPack reference and consultation in one transaction. A crash may leave only unreferenced chunks, which compaction removes after a grace period. If metadata references a missing chunk or one with the wrong size or digest, store opening and ContextPack reads fail.

Writing the same payload under the same ContextPack ID succeeds idempotently; a different payload conflicts. Consultation creation also validates the idempotency scope and fingerprint.

ContextPack reads and writes hold a shared chunk-lifecycle lock, while destructive compaction holds it exclusively. This prevents a compactor from deleting a deduplicated chunk just before metadata links it, or removing a live chunk during a read.

## v1 and v2 Migration

Earlier stores embedded ContextPack source in metadata JSON. v3 opens them in this order:

```text
decode legacy JSON
-> write ContextPack content chunks
-> verify all references
-> retain original as .v2.bak
-> atomically replace with v3 metadata
```

Opening does not succeed if backup creation or v3 publication fails. Never delete the backup automatically. v3 recomputes SHA-256 for every chunk, rejecting same-size content tampering during startup.

## Compaction

`expertstorectl compact` removes data in this order:

1. Terminal consultations older than retention
2. Idempotency records not referenced by consultations
3. Optional orphan ContextPacks and results after the grace period
4. Orphan chunks after computing the final live set and grace period

```powershell
go run ./cmd/expertstorectl stats
go run ./cmd/expertstorectl compact --dry-run
go run ./cmd/expertstorectl compact `
  --terminal-retention 720h `
  --orphan-grace 24h `
  --remove-orphans=true
```

`--dry-run` changes neither metadata nor chunks. Do not use a zero grace period by default; it can immediately remove chunks left by a newly interrupted write.

## Failure Boundary

- Keep metadata and chunks on the same local filesystem.
- Create store files and chunks with user-only permissions.
- Network-share rename and flush semantics are unsupported.
- If antivirus temporarily blocks a chunk rename, consultation creation fails rather than returning empty success.
- Editing a source file does not change existing ContextPack chunks; new work receives a new digest and ContextPack ID.
