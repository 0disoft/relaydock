package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/idgen"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/compiler"
)

type Adapter struct{}

func New() *Adapter                           { return &Adapter{} }
func (*Adapter) Protocol() canonical.Protocol { return canonical.ProtocolOpenAIChat }

type request struct {
	Model               string            `json:"model"`
	Messages            []message         `json:"messages"`
	Tools               json.RawMessage   `json:"tools,omitempty"`
	ResponseFormat      json.RawMessage   `json:"response_format,omitempty"`
	MaxTokens           int64             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int64             `json:"max_completion_tokens,omitempty"`
	Stream              bool              `json:"stream,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
}
type message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	Name       string          `json:"name,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []toolCall      `json:"tool_calls,omitempty"`
}
type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type,omitempty"`
	Function functionCall `json:"function"`
}
type functionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type contentPart struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ImageURL  any    `json:"image_url,omitempty"`
	InputText string `json:"input_text,omitempty"`
}

func (*Adapter) Decode(body []byte) (canonical.RequestEnvelope, error) {
	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		return canonical.RequestEnvelope{}, fmt.Errorf("%w: %v", core.ErrInvalidArgument, err)
	}
	if len(req.Messages) == 0 {
		return canonical.RequestEnvelope{}, fmt.Errorf("%w: messages are required", core.ErrInvalidArgument)
	}
	e := canonical.RequestEnvelope{RequestID: idgen.New("req"), IngressProtocol: canonical.ProtocolOpenAIChat, Model: req.Model, Stream: req.Stream, Extensions: map[string]json.RawMessage{}, Metadata: req.Metadata}
	e.Require(canonical.CapabilityTextInput)
	if req.MaxCompletionTokens > 0 {
		e.Requirements.MaximumOutputTokens = req.MaxCompletionTokens
	} else {
		e.Requirements.MaximumOutputTokens = req.MaxTokens
	}
	if len(req.Tools) > 0 && !bytes.Equal(req.Tools, []byte("null")) {
		e.Extensions["openai.chat.tools"] = cloneRaw(req.Tools)
		e.Require(canonical.CapabilityToolCalling)
	}
	if len(req.ResponseFormat) > 0 && !bytes.Equal(req.ResponseFormat, []byte("null")) {
		e.Extensions["openai.chat.response_format"] = cloneRaw(req.ResponseFormat)
		e.Require(canonical.CapabilityStructuredOutput)
	}
	for _, m := range req.Messages {
		role := canonical.Role(m.Role)
		parts, image, err := decodeContent(m.Content)
		if err != nil {
			return canonical.RequestEnvelope{}, err
		}
		if image {
			e.Require(canonical.CapabilityImageInput)
		}
		if m.Role == "tool" {
			e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemToolResult, Role: canonical.RoleTool, ToolResult: &canonical.ToolResult{ToolCallID: m.ToolCallID, Content: parts}})
			continue
		}
		if len(parts) > 0 {
			e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemMessage, Role: role, Content: parts})
		}
		for _, tc := range m.ToolCalls {
			args := tc.Function.Arguments
			if len(args) == 0 {
				args = []byte("{}")
			}
			e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemToolCall, Role: canonical.RoleAssistant, ToolCall: &canonical.ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: cloneRaw(args)}})
			e.Require(canonical.CapabilityToolCalling)
		}
	}
	return e, nil
}

func (*Adapter) Encode(e canonical.RequestEnvelope, mode compiler.LossMode) ([]byte, compiler.Report, error) {
	report := compiler.NewReport(mode)
	req := request{Model: e.Model, Stream: e.Stream, Metadata: e.Metadata, MaxCompletionTokens: e.Requirements.MaximumOutputTokens}
	if raw := e.Extensions["openai.chat.tools"]; len(raw) > 0 {
		req.Tools = cloneRaw(raw)
	}
	if raw := e.Extensions["openai.chat.response_format"]; len(raw) > 0 {
		req.ResponseFormat = cloneRaw(raw)
	}
	for i, item := range e.Items {
		switch item.Kind {
		case canonical.ItemMessage:
			content, err := encodeContent(item.Content)
			if err != nil {
				return nil, report, err
			}
			req.Messages = append(req.Messages, message{Role: string(item.Role), Content: content})
		case canonical.ItemToolCall:
			if item.ToolCall == nil {
				continue
			}
			args := item.ToolCall.Arguments
			if len(args) == 0 {
				args = []byte("{}")
			}
			req.Messages = append(req.Messages, message{Role: "assistant", Content: json.RawMessage("null"), ToolCalls: []toolCall{{ID: item.ToolCall.ID, Type: "function", Function: functionCall{Name: item.ToolCall.Name, Arguments: cloneRaw(args)}}}})
		case canonical.ItemToolResult:
			if item.ToolResult == nil {
				continue
			}
			content, err := encodeContent(item.ToolResult.Content)
			if err != nil {
				return nil, report, err
			}
			req.Messages = append(req.Messages, message{Role: "tool", ToolCallID: item.ToolResult.ToolCallID, Content: content})
		case canonical.ItemReasoning:
			report.Add(fmt.Sprintf("items[%d].reasoning", i), e.IngressProtocol, canonical.ProtocolOpenAIChat, "Chat Completions cannot preserve a reasoning item or its signature")
		default:
			return nil, report, fmt.Errorf("%w: item kind %s", core.ErrInvalidArgument, item.Kind)
		}
	}
	if report.HasFatalLoss() {
		return nil, report, core.ErrLossyTransformation
	}
	body, err := json.Marshal(req)
	return body, report, err
}

func decodeContent(raw json.RawMessage) ([]canonical.ContentPart, bool, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, false, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []canonical.ContentPart{{Type: "text", Text: text}}, false, nil
	}
	var parts []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, false, fmt.Errorf("%w: invalid message content", core.ErrInvalidArgument)
	}
	out := make([]canonical.ContentPart, 0, len(parts))
	image := false
	for _, p := range parts {
		var typ string
		_ = json.Unmarshal(p["type"], &typ)
		switch typ {
		case "text", "input_text":
			var t string
			if typ == "input_text" {
				_ = json.Unmarshal(p["input_text"], &t)
			} else {
				_ = json.Unmarshal(p["text"], &t)
			}
			out = append(out, canonical.ContentPart{Type: "text", Text: t})
		case "image_url", "input_image":
			var uri string
			if v := p["image_url"]; len(v) > 0 {
				var s string
				if json.Unmarshal(v, &s) == nil {
					uri = s
				} else {
					var obj struct {
						URL string `json:"url"`
					}
					_ = json.Unmarshal(v, &obj)
					uri = obj.URL
				}
			}
			if uri == "" {
				_ = json.Unmarshal(p["image_url"], &uri)
			}
			out = append(out, canonical.ContentPart{Type: "image", URI: uri})
			image = true
		default:
			out = append(out, canonical.ContentPart{Type: typ})
		}
	}
	return out, image, nil
}
func encodeContent(parts []canonical.ContentPart) (json.RawMessage, error) {
	if len(parts) == 1 && parts[0].Type == "text" {
		return json.Marshal(parts[0].Text)
	}
	values := make([]map[string]any, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			values = append(values, map[string]any{"type": "text", "text": p.Text})
		case "image":
			values = append(values, map[string]any{"type": "image_url", "image_url": map[string]string{"url": p.URI}})
		default:
			return nil, fmt.Errorf("%w: unsupported content type %s", core.ErrCapabilityMismatch, p.Type)
		}
	}
	return json.Marshal(values)
}
func cloneRaw(v json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), v...) }
func normalizeRole(role canonical.Role) string {
	v := strings.TrimSpace(string(role))
	if v == "" {
		return "user"
	}
	return v
}
