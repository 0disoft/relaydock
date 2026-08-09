# 07. ContextPack

## Contents

- Objective
- Success criteria
- Repository revision
- Working-tree digest
- Architecture summary
- Relevant file fragments
- Relevant tests
- Error logs
- Previously failed approaches
- Forbidden alternatives
- Open questions

## File Selection

The selector combines these signals:

- Explicitly mentioned files
- Failing-test import graph
- Git diff
- Symbol references
- Runtime stack traces
- Architecture ownership map
- User pins

## Redaction

Run locally before transmission:

- Path denylist
- Default exclusion of `.env`, keys, and certificates
- Known token prefixes
- High-entropy values
- Connection strings
- Personal-data patterns
- Custom user rules

## Integrity

Record a digest and line range for every item of evidence. Compare current file digests when consultation results arrive and mark stale evidence.

## Limits

- Maximum file count
- Maximum bytes
- Maximum estimated tokens
- Exclude binaries
- Exclude generated code by default
- Exclude vendored dependencies by default
