# ADR-0004: No ChatGPT Web Automation

- Status: Accepted
- Date: 2026-08-06
- Owners: Expert Escalation, Security

## Context

Automating web-only models through browser DOMs, cookies, or private endpoints makes the entire product depend on UI changes and account policy. It also increases the risks of credential theft, terms violations, account suspension, and forged response provenance.

## Decision

The official product does not provide DOM scraping, remote browser control, session-cookie extraction, or private-endpoint calls. Web-model access is a handoff in which the user opens the consultation in ChatGPT and submits a structured result. Automated routes use official API adapters only.

## Product Contract

Record web-handoff model provenance as `user_declared`. Do not label it as verified by a provider response like an API result. Handoff tokens have short lifetimes, consultation-specific scopes, and reuse limits.

## Consequences

Personal accounts retain one user action, but browser changes cannot disable the entire service. If a future official Workspace API supplies the required response and model attestation, add automation through a separate adapter and ADR.
