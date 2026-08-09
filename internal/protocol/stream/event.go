package stream

import "encoding/json"

type EventKind string

const (
	EventResponseStarted EventKind = "response.started"
	EventContentDelta    EventKind = "content.delta"
	EventToolCallDelta   EventKind = "tool_call.delta"
	EventReasoningDelta  EventKind = "reasoning.delta"
	EventUsage           EventKind = "usage"
	EventProviderRaw     EventKind = "provider.raw"
	EventCompleted       EventKind = "response.completed"
	EventFailed          EventKind = "response.failed"
)

type Event struct {
	Sequence  int64
	Kind      EventKind
	ItemID    string
	Delta     []byte
	Usage     *Usage
	Extension map[string]json.RawMessage
}
type Usage struct {
	InputTokens      int64 `json:"inputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	ReasoningTokens  int64 `json:"reasoningTokens"`
}

func (u *Usage) Add(v Usage) {
	u.InputTokens += v.InputTokens
	u.CacheReadTokens += v.CacheReadTokens
	u.CacheWriteTokens += v.CacheWriteTokens
	u.OutputTokens += v.OutputTokens
	u.ReasoningTokens += v.ReasoningTokens
}
