# 08. Protocol Compiler

## Canonical Representation

Represent shared concepts as canonical items:

- message
- text
- image
- tool call
- tool result
- reasoning
- refusal
- usage
- provider extension

Preserve provider-specific fields in `ProviderExtensions`.

## Conversion Modes

### Strict

Reject a request when any potential semantic loss exists.

### Compatible

Allow only prevalidated conversions and return a loss report.

### Passthrough

Preserve the original meaning for the same protocol or an explicitly wire-compatible upstream.

## Loss Kinds

- field dropped
- field approximated
- ordering changed
- tool identity remapped
- reasoning hidden
- state continuation unavailable
- cache semantics changed
- modality unavailable
- unknown event preserved
- unknown event rejected

## Prohibition

Do not use a single `messages[]` shape as a universal internal model. Do not force Responses items, Anthropic blocks, and Gemini parts into an artificial flat representation.
