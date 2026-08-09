# ADR-0008: License Boundary

- Status: Accepted
- Date: 2026-08-09
- Owners: Product, Legal, Open Source

## Context

RelayDock aims to be an open AI runtime that can replace projects in the sub2api and OpenCodex category. Declaring only selected paths private within the same repository would make contributor and distribution rights ambiguous and make it difficult to verify automatically which terms apply to source archives and containers.

## Decision

Publish all source, documentation, generated contracts, and RelayDock-distributed artifacts in this repository under Apache License 2.0. The root `LICENSE` is the sole repository license, and package metadata uses `Apache-2.0`.

If 0disoft later develops hosted secret custody, private abuse policy, internal operational data, or proprietary deployment automation, keep it in a separate private repository rather than creating an exception directory such as `enterprise/` here. Integrate with open RelayDock only through versioned public APIs and protocol contracts.

## Constraints

- Track third-party licenses and NOTICE obligations in the SBOM.
- Do not add resale of upstream consumer accounts to the public repository.
- Review Apache-2.0 compatibility and attribution duties when adding dependencies or external materials.
- `NOTICE` preserves project attribution; update required third-party notices from release-SBOM review results.

## Consequences

- Source and binaries can ship under the same terms, and the default contribution terms follow Apache-2.0 section 5.
- Apache-2.0 does not grant trademark rights, so project name and logo policies may be defined separately.
- A CLA, DCO, or separate trademark policy may be added later, but licensing uncertainty no longer blocks public distribution.
