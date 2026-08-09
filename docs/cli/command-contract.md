# CLI Contract Route

- Status: Active index
- Authority: command implementation and its owning contract document

CLIs under `cmd/` own only composition and process lifecycle; they do not duplicate domain rules. See [`../03-repository-map.md`](../03-repository-map.md) for the executable inventory and responsibilities, and [`../15-build-and-release.md`](../15-build-and-release.md) for release inclusion.

When changing CLI behavior, review all of the following:

- Flags, defaults, stdout/stderr, and exit semantics
- Backward compatibility of JSON output and exposure of secrets or file contents
- The owning domain contract and operations runbook
- Related tests and the actual verification scope in [`../../VALIDATION.md`](../../VALIDATION.md)

This document grants no permission to run shell commands. Agents may run only one-shot intents registered in the parent mustflow contract.
