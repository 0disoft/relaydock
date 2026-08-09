# 17. Open Questions

Remove items from this page once code decides them. Close each question through an ADR or operational-policy revision before implementation.

## Product and Packaging

- Operational boundary between a separate private managed-service repository and the public RelayDock API
- Commercial terms for enterprise self-hosting support, warranties, and trademark use
- Provider, route, and Expert-call limits for the free personal edition

## Wails Distribution

- Default Windows installation format: NSIS or MSIX
- Ownership of actual process replacement and rollback in the updater
- Scope of macOS notarization and entitlements
- Target Linux distributions and minimum WebKitGTK baseline
- Release gate for removing the Wails v3 alpha pin

## MCP and Expert

- OAuth/OIDC issuer and organization RBAC model for Remote MCP
- Result-import UX for personal ChatGPT plans
- UI representation of user-declared model attestation
- Whether customers or the platform absorb failed API Expert costs
- Maximum delegation and cost policy for multi-Expert panels

## ContextPack

- Whether the symbol index should use Go parsers or tree-sitter
- Default exclusion and manual inclusion policy for gitignored files
- When to add provider-specific tokenizers
- Summary adapters for binary artifacts, images, and large logs
- Encryption-key ownership and TTL for R2/S3 uploads

## Gateway and Routing

- When to make signed Control snapshots the only production source of truth
- Data model for sharing process-local cooldowns through Valkey
- Provider health-probe interval and false-positive suppression
- Personal-data boundaries for session-affinity and prompt-cache-affinity keys
- Provider allowlist for same-provider stream resume

## Accounting

- Customer-charge policy for failed attempts
- Maximum delay for provider-usage corrections
- Exchange-rate and Mandarin price-revision timing
- Consumption order for free, paid, and promotional credits
- Minimum adjustment-ledger retention and audit-export format

## Infrastructure and Operations

- Schema-level ACLs for the initial single PostgreSQL cluster
- Default managed object store: R2, B2, or S3
- Provider egress regions and data residency
- Throughput limit for PostgreSQL polling outbox and broker-adoption trigger
- Single-region recovery objectives and multi-region entry conditions
