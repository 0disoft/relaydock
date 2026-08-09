package messages

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/idgen"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/compiler"
)

type Adapter struct{}

func New() *Adapter                           { return &Adapter{} }
func (*Adapter) Protocol() canonical.Protocol { return canonical.ProtocolAnthropicMessages }

type request struct {
	Model     string          `json:"model"`
	System    json.RawMessage `json:"system,omitempty"`
	Messages  []message       `json:"messages"`
	Tools     json.RawMessage `json:"tools,omitempty"`
	MaxTokens int64           `json:"max_tokens"`
	Stream    bool            `json:"stream,omitempty"`
	Metadata  map[string]any  `json:"metadata,omitempty"`
	Thinking  json.RawMessage `json:"thinking,omitempty"`
}
type message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}
type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	Source    *source         `json:"source,omitempty"`
}
type source struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
	URL       string `json:"url,omitempty"`
}

func (*Adapter) Decode(body []byte) (canonical.RequestEnvelope, error) {
	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		return canonical.RequestEnvelope{}, fmt.Errorf("%w: %v", core.ErrInvalidArgument, err)
	}
	e := canonical.RequestEnvelope{RequestID: idgen.New("req"), IngressProtocol: canonical.ProtocolAnthropicMessages, Model: req.Model, Stream: req.Stream, Extensions: map[string]json.RawMessage{}}
	e.Require(canonical.CapabilityTextInput)
	e.Requirements.MaximumOutputTokens = req.MaxTokens
	if len(req.Tools) > 0 && !bytes.Equal(req.Tools, []byte("null")) {
		e.Extensions["anthropic.tools"] = append([]byte(nil), req.Tools...)
		e.Require(canonical.CapabilityToolCalling)
	}
	if len(req.Thinking) > 0 && !bytes.Equal(req.Thinking, []byte("null")) {
		e.Extensions["anthropic.thinking"] = append([]byte(nil), req.Thinking...)
		e.Require(canonical.CapabilityReasoning)
	}
	if len(req.System) > 0 && !bytes.Equal(req.System, []byte("null")) {
		parts, _, err := decodeBlocks(req.System)
		if err != nil {
			return canonical.RequestEnvelope{}, err
		}
		e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemMessage, Role: canonical.RoleSystem, Content: parts})
	}
	for _, m := range req.Messages {
		var blocks []block
		if err := json.Unmarshal(m.Content, &blocks); err != nil {
			var text string
			if json.Unmarshal(m.Content, &text) == nil {
				blocks = []block{{Type: "text", Text: text}}
			} else {
				return canonical.RequestEnvelope{}, fmt.Errorf("%w: invalid Anthropic content", core.ErrInvalidArgument)
			}
		}
		for _, b := range blocks {
			switch b.Type {
			case "text":
				e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemMessage, Role: canonical.Role(m.Role), Content: []canonical.ContentPart{{Type: "text", Text: b.Text}}})
			case "image":
				uri := ""
				mime := ""
				if b.Source != nil {
					mime = b.Source.MediaType
					if b.Source.URL != "" {
						uri = b.Source.URL
					} else if b.Source.Data != "" {
						uri = "data:" + mime + ";base64," + b.Source.Data
					}
				}
				e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemMessage, Role: canonical.Role(m.Role), Content: []canonical.ContentPart{{Type: "image", URI: uri, MIME: mime}}})
				e.Require(canonical.CapabilityImageInput)
			case "tool_use":
				e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemToolCall, Role: canonical.RoleAssistant, ToolCall: &canonical.ToolCall{ID: b.ID, Name: b.Name, Arguments: append([]byte(nil), b.Input...)}})
				e.Require(canonical.CapabilityToolCalling)
			case "tool_result":
				parts, _, err := decodeBlocks(b.Content)
				if err != nil {
					return canonical.RequestEnvelope{}, err
				}
				e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemToolResult, Role: canonical.RoleTool, ToolResult: &canonical.ToolResult{ToolCallID: b.ToolUseID, Content: parts, IsError: b.IsError}})
			case "thinking":
				opaque, _ := json.Marshal(b)
				e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemReasoning, Role: canonical.RoleAssistant, Reasoning: &canonical.ReasoningBlock{Summary: b.Thinking, Signature: b.Signature, Opaque: opaque}})
				e.Require(canonical.CapabilityReasoning)
			default:
				return canonical.RequestEnvelope{}, fmt.Errorf("%w: Anthropic block %q", core.ErrCapabilityMismatch, b.Type)
			}
		}
	}
	return e, nil
}
func (*Adapter) Encode(e canonical.RequestEnvelope, mode compiler.LossMode) ([]byte, compiler.Report, error) {
	report := compiler.NewReport(mode)
	req := request{Model: e.Model, MaxTokens: e.Requirements.MaximumOutputTokens, Stream: e.Stream}
	if req.MaxTokens <= 0 {
		req.MaxTokens = 1024
	}
	req.Tools = append([]byte(nil), e.Extensions["anthropic.tools"]...)
	req.Thinking = append([]byte(nil), e.Extensions["anthropic.thinking"]...)
	var system []block
	messages := make([]message, 0)
	for i, it := range e.Items {
		switch it.Kind {
		case canonical.ItemMessage:
			blocks, err := encodeCanonicalParts(it.Content)
			if err != nil {
				return nil, report, err
			}
			if it.Role == canonical.RoleSystem {
				system = append(system, blocks...)
			} else {
				raw, _ := json.Marshal(blocks)
				messages = append(messages, message{Role: string(it.Role), Content: raw})
			}
		case canonical.ItemToolCall:
			if it.ToolCall != nil {
				raw, _ := json.Marshal([]block{{Type: "tool_use", ID: it.ToolCall.ID, Name: it.ToolCall.Name, Input: append([]byte(nil), it.ToolCall.Arguments...)}})
				messages = append(messages, message{Role: "assistant", Content: raw})
			}
		case canonical.ItemToolResult:
			if it.ToolResult != nil {
				blocks, err := encodeCanonicalParts(it.ToolResult.Content)
				if err != nil {
					return nil, report, err
				}
				content, _ := json.Marshal(blocks)
				raw, _ := json.Marshal([]block{{Type: "tool_result", ToolUseID: it.ToolResult.ToolCallID, Content: content, IsError: it.ToolResult.IsError}})
				messages = append(messages, message{Role: "user", Content: raw})
			}
		case canonical.ItemReasoning:
			if e.IngressProtocol == canonical.ProtocolAnthropicMessages && it.Reasoning != nil && len(it.Reasoning.Opaque) > 0 {
				raw, _ := json.Marshal([]json.RawMessage{it.Reasoning.Opaque})
				messages = append(messages, message{Role: "assistant", Content: raw})
			} else {
				report.Add(fmt.Sprintf("items[%d].reasoning", i), e.IngressProtocol, canonical.ProtocolAnthropicMessages, "Anthropic thinking signatures cannot be synthesized across providers")
			}
		}
	}
	if len(system) > 0 {
		req.System, _ = json.Marshal(system)
	}
	req.Messages = messages
	if report.HasFatalLoss() {
		return nil, report, core.ErrLossyTransformation
	}
	body, err := json.Marshal(req)
	return body, report, err
}
func decodeBlocks(raw json.RawMessage) ([]canonical.ContentPart, bool, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []canonical.ContentPart{{Type: "text", Text: text}}, false, nil
	}
	var blocks []block
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, false, fmt.Errorf("%w: invalid content block", core.ErrInvalidArgument)
	}
	parts := make([]canonical.ContentPart, 0, len(blocks))
	image := false
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, canonical.ContentPart{Type: "text", Text: b.Text})
		case "image":
			uri := ""
			mime := ""
			if b.Source != nil {
				uri = b.Source.URL
				mime = b.Source.MediaType
				if uri == "" && b.Source.Data != "" {
					uri = "data:" + mime + ";base64," + b.Source.Data
				}
			}
			parts = append(parts, canonical.ContentPart{Type: "image", URI: uri, MIME: mime})
			image = true
		default:
			parts = append(parts, canonical.ContentPart{Type: b.Type})
		}
	}
	return parts, image, nil
}
func encodeCanonicalParts(parts []canonical.ContentPart) ([]block, error) {
	out := make([]block, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			out = append(out, block{Type: "text", Text: p.Text})
		case "image":
			out = append(out, block{Type: "image", Source: &source{Type: "url", URL: p.URI, MediaType: p.MIME}})
		default:
			return nil, fmt.Errorf("%w: content type %s", core.ErrCapabilityMismatch, p.Type)
		}
	}
	return out, nil
}
