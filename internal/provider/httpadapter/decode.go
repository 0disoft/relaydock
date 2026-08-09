package httpadapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/stream"
)

func DecodePayload(providerName string, protocol canonical.Protocol, raw []byte) []stream.Event {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return []stream.Event{{Kind: stream.EventProviderRaw, Delta: append([]byte(nil), raw...)}}
	}
	events := []stream.Event{}
	typ, _ := root["type"].(string)
	switch typ {
	case "response.created", "response.in_progress", "message_start":
		events = append(events, stream.Event{Kind: stream.EventResponseStarted})
	case "response.output_text.delta":
		events = append(events, stream.Event{Kind: stream.EventContentDelta, Delta: []byte(stringValue(root["delta"]))})
	case "response.reasoning_summary_text.delta":
		events = append(events, stream.Event{Kind: stream.EventReasoningDelta, Delta: []byte(stringValue(root["delta"]))})
	case "response.function_call_arguments.delta":
		events = append(events, stream.Event{Kind: stream.EventToolCallDelta, ItemID: stringValue(root["item_id"]), Delta: []byte(stringValue(root["delta"]))})
	case "response.completed", "message_stop":
		if u := ExtractUsage(raw); !zeroUsage(u) {
			events = append(events, stream.Event{Kind: stream.EventUsage, Usage: &u})
		}
		events = append(events, stream.Event{Kind: stream.EventCompleted})
	case "response.failed", "error":
		events = append(events, stream.Event{Kind: stream.EventFailed, Delta: append([]byte(nil), raw...)})
	case "content_block_delta":
		if delta, ok := root["delta"].(map[string]any); ok {
			switch stringValue(delta["type"]) {
			case "text_delta":
				events = append(events, stream.Event{Kind: stream.EventContentDelta, Delta: []byte(stringValue(delta["text"]))})
			case "input_json_delta":
				events = append(events, stream.Event{Kind: stream.EventToolCallDelta, Delta: []byte(stringValue(delta["partial_json"]))})
			case "thinking_delta":
				events = append(events, stream.Event{Kind: stream.EventReasoningDelta, Delta: []byte(stringValue(delta["thinking"]))})
			}
		}
	case "message_delta":
		if u := ExtractUsage(raw); !zeroUsage(u) {
			events = append(events, stream.Event{Kind: stream.EventUsage, Usage: &u})
		}
	}
	if choices, ok := root["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if delta, ok := choice["delta"].(map[string]any); ok {
				if text := stringValue(delta["content"]); text != "" {
					events = append(events, stream.Event{Kind: stream.EventContentDelta, Delta: []byte(text)})
				}
				if calls, ok := delta["tool_calls"].([]any); ok {
					for _, rawCall := range calls {
						b, _ := json.Marshal(rawCall)
						events = append(events, stream.Event{Kind: stream.EventToolCallDelta, Delta: b})
					}
				}
			}
			if choice["finish_reason"] != nil {
				if u := ExtractUsage(raw); !zeroUsage(u) {
					events = append(events, stream.Event{Kind: stream.EventUsage, Usage: &u})
				}
				events = append(events, stream.Event{Kind: stream.EventCompleted})
			}
		}
	}
	if len(events) == 0 {
		text := extractText(root)
		usage := ExtractUsage(raw)
		if typ != "" && text == "" && zeroUsage(usage) {
			return []stream.Event{{
				Kind:  stream.EventProviderRaw,
				Delta: append([]byte(nil), raw...),
				Extension: map[string]json.RawMessage{
					"provider": json.RawMessage(fmt.Sprintf("%q", providerName)),
					"type":     json.RawMessage(fmt.Sprintf("%q", typ)),
				},
			}}
		}
		if text != "" {
			events = append(events, stream.Event{Kind: stream.EventResponseStarted}, stream.Event{Kind: stream.EventContentDelta, Delta: []byte(text)})
		}
		if !zeroUsage(usage) {
			events = append(events, stream.Event{Kind: stream.EventUsage, Usage: &usage})
		}
		events = append(events, stream.Event{Kind: stream.EventCompleted, Extension: map[string]json.RawMessage{"provider": json.RawMessage(fmt.Sprintf("%q", providerName)), "raw": append([]byte(nil), raw...)}})
	}
	return events
}
func ExtractUsage(raw []byte) stream.Usage {
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return stream.Usage{}
	}
	usage := mapValue(root["usage"])
	if len(usage) == 0 {
		usage = mapValue(root["usageMetadata"])
	}
	input := intValue(first(usage, "input_tokens", "inputTokens", "promptTokenCount"))
	output := intValue(first(usage, "output_tokens", "outputTokens", "candidatesTokenCount"))
	cacheRead := intValue(first(usage, "cache_read_input_tokens", "cached_tokens", "cachedContentTokenCount"))
	cacheWrite := intValue(first(usage, "cache_creation_input_tokens", "cache_write_input_tokens"))
	reasoning := intValue(first(usage, "reasoning_tokens", "thoughtsTokenCount"))
	if details := mapValue(usage["input_tokens_details"]); len(details) > 0 {
		cacheRead = maxInt(cacheRead, intValue(details["cached_tokens"]))
	}
	if details := mapValue(usage["output_tokens_details"]); len(details) > 0 {
		reasoning = maxInt(reasoning, intValue(details["reasoning_tokens"]))
	}
	return stream.Usage{InputTokens: input, OutputTokens: output, CacheReadTokens: cacheRead, CacheWriteTokens: cacheWrite, ReasoningTokens: reasoning}
}
func extractText(root map[string]any) string {
	if v := stringValue(root["output_text"]); v != "" {
		return v
	}
	if choices, ok := root["choices"].([]any); ok && len(choices) > 0 {
		if c, ok := choices[0].(map[string]any); ok {
			if m := mapValue(c["message"]); len(m) > 0 {
				return stringValue(m["content"])
			}
		}
	}
	if content, ok := root["content"].([]any); ok {
		var b strings.Builder
		for _, v := range content {
			m := mapValue(v)
			if stringValue(m["type"]) == "text" {
				b.WriteString(stringValue(m["text"]))
			}
		}
		return b.String()
	}
	if candidates, ok := root["candidates"].([]any); ok && len(candidates) > 0 {
		c := mapValue(candidates[0])
		content := mapValue(c["content"])
		if parts, ok := content["parts"].([]any); ok {
			var b strings.Builder
			for _, v := range parts {
				b.WriteString(stringValue(mapValue(v)["text"]))
			}
			return b.String()
		}
	}
	return ""
}
func stringValue(v any) string      { s, _ := v.(string); return s }
func mapValue(v any) map[string]any { m, _ := v.(map[string]any); return m }
func intValue(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}
func first(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}
func maxInt(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func zeroUsage(u stream.Usage) bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.CacheReadTokens == 0 && u.CacheWriteTokens == 0 && u.ReasoningTokens == 0
}
