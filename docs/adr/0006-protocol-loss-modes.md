# ADR-0006: Protocol Loss Modes

- Status: Accepted
- Date: 2026-08-06
- Owners: Protocol Compiler

## Context

OpenAI Responses, Chat Completions, Anthropic Messages, and Gemini represent reasoning, tool-call deltas, continuation, and cache metadata differently. Flattening every request to the lowest common denominator causes silent capability loss and responses that cannot be debugged.

## Decision

Provide exactly three conversion modes: `strict`, `compatible`, and `passthrough`.

- `strict`: reject requests when semantic or provider-specific data may be lost.
- `compatible`: allow only explicitly validated conversions and record a loss report.
- `passthrough`: prioritize preservation of the original form when ingress and upstream are identical or fully compatible.

Do not provide a silent best-effort mode. An adapter that discards unsupported fields and returns success fails conformance.

## Validation

Every conversion pair requires golden request, response, and stream fixtures; unknown-event preservation; tool-call fragment and reasoning-item coverage; and cancellation and partial-EOF tests. A new protocol capability must add both a capability-registry entry and a loss rule.
