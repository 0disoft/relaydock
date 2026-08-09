# 00. Product Identity

## One-Sentence Definition

An AI Runtime Gateway that lets coding agents use fast models for routine work, send only **verifiable context** through a stronger expert path for difficult architecture, security, billing, or migration decisions, and bring the result back into implementation.

## Differentiation

This product is not trying to be a proxy with the largest model catalog.

Its five core assets are:

1. A protocol compiler that preserves provider-specific reasoning, tool, and stream semantics
2. Retry semantics that distinguish failures before and after the first token
3. A ContextPack compiler that bundles code, tests, and failure history
4. A consultation contract that turns expert results back into implementation and verification conditions
5. An accounting structure that separates provider usage from customer charges

## Target Users

- Individual developers who use Codex, Claude Code, or OpenCode extensively
- Small teams that mix multiple providers and self-hosted models
- Product organizations that need centralized control over model cost and permissions
- Indie hackers who expose AI APIs through their own products

## Product Surfaces

One brand provides three product surfaces:

| Surface | Installation | Core value |
|---|---|---|
| Desktop Runtime | User workstation | Local keys, MCP, ContextPack, and expert handoff |
| Managed Gateway | Server | Virtual keys, protocol translation, routing, and usage |
| Control Console | Web | Organizations, policies, cost, and audit |

The UI connects these surfaces, but their trust boundaries and deployment artifacts remain separate.

## Success Criteria

MVP success is not measured by provider count.

- Explain which semantics are lost during provider conversion for the same input.
- Never record a midstream failure as success.
- Reproduce the files and revision sent to an expert.
- Never capture the same usage twice.
- Keep the desktop agent attached reliably without restarting Codex.
