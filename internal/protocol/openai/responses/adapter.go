package responses

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
func (*Adapter) Protocol() canonical.Protocol { return canonical.ProtocolOpenAIResponses }

type request struct {
	Model              string            `json:"model"`
	Input              json.RawMessage   `json:"input"`
	Instructions       string            `json:"instructions,omitempty"`
	Tools              json.RawMessage   `json:"tools,omitempty"`
	Reasoning          json.RawMessage   `json:"reasoning,omitempty"`
	Text               json.RawMessage   `json:"text,omitempty"`
	PreviousResponseID string            `json:"previous_response_id,omitempty"`
	MaxOutputTokens    int64             `json:"max_output_tokens,omitempty"`
	Stream             bool              `json:"stream,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

type inputItem struct {
	Type             string          `json:"type"`
	Role             string          `json:"role,omitempty"`
	Content          json.RawMessage `json:"content,omitempty"`
	CallID           string          `json:"call_id,omitempty"`
	Name             string          `json:"name,omitempty"`
	Arguments        json.RawMessage `json:"arguments,omitempty"`
	Output           json.RawMessage `json:"output,omitempty"`
	Summary          json.RawMessage `json:"summary,omitempty"`
	EncryptedContent string          `json:"encrypted_content,omitempty"`
}

type part struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
}

func (*Adapter) Decode(body []byte) (canonical.RequestEnvelope, error) {
	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		return canonical.RequestEnvelope{}, fmt.Errorf("%w: %v", core.ErrInvalidArgument, err)
	}
	e := canonical.RequestEnvelope{RequestID: idgen.New("req"), IngressProtocol: canonical.ProtocolOpenAIResponses, Model: req.Model, Stream: req.Stream, Extensions: map[string]json.RawMessage{}, Metadata: req.Metadata}
	e.Require(canonical.CapabilityTextInput)
	e.Requirements.MaximumOutputTokens = req.MaxOutputTokens
	if req.Instructions != "" {
		e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemMessage, Role: canonical.RoleSystem, Content: []canonical.ContentPart{{Type: "text", Text: req.Instructions}}})
	}
	if len(req.Tools) > 0 && !bytes.Equal(req.Tools, []byte("null")) {
		e.Extensions["openai.responses.tools"] = append([]byte(nil), req.Tools...)
		e.Require(canonical.CapabilityToolCalling)
	}
	if len(req.Reasoning) > 0 && !bytes.Equal(req.Reasoning, []byte("null")) {
		e.Extensions["openai.responses.reasoning"] = append([]byte(nil), req.Reasoning...)
		e.Require(canonical.CapabilityReasoning)
	}
	if len(req.Text) > 0 && !bytes.Equal(req.Text, []byte("null")) {
		e.Extensions["openai.responses.text"] = append([]byte(nil), req.Text...)
		e.Require(canonical.CapabilityStructuredOutput)
	}
	if req.PreviousResponseID != "" {
		e.Extensions["openai.responses.previous_response_id"], _ = json.Marshal(req.PreviousResponseID)
		e.Require(canonical.CapabilityContinuation)
	}
	var scalar string
	if json.Unmarshal(req.Input, &scalar) == nil {
		e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemMessage, Role: canonical.RoleUser, Content: []canonical.ContentPart{{Type: "text", Text: scalar}}})
		return e, nil
	}
	var items []inputItem
	if err := json.Unmarshal(req.Input, &items); err != nil {
		return canonical.RequestEnvelope{}, fmt.Errorf("%w: invalid Responses input", core.ErrInvalidArgument)
	}
	for _, it := range items {
		switch it.Type {
		case "message", "":
			parts, image, err := decodeParts(it.Content)
			if err != nil {
				return canonical.RequestEnvelope{}, err
			}
			if image {
				e.Require(canonical.CapabilityImageInput)
			}
			e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemMessage, Role: canonical.Role(it.Role), Content: parts})
		case "function_call":
			e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemToolCall, Role: canonical.RoleAssistant, ToolCall: &canonical.ToolCall{ID: it.CallID, Name: it.Name, Arguments: append([]byte(nil), it.Arguments...)}})
			e.Require(canonical.CapabilityToolCalling)
		case "function_call_output":
			parts, _, err := decodeParts(it.Output)
			if err != nil {
				var text string
				if json.Unmarshal(it.Output, &text) == nil {
					parts = []canonical.ContentPart{{Type: "text", Text: text}}
				} else {
					return canonical.RequestEnvelope{}, err
				}
			}
			e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemToolResult, Role: canonical.RoleTool, ToolResult: &canonical.ToolResult{ToolCallID: it.CallID, Content: parts}})
		case "reasoning":
			e.Items = append(e.Items, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemReasoning, Role: canonical.RoleAssistant, Reasoning: &canonical.ReasoningBlock{Opaque: append([]byte(nil), it.Summary...)}})
			e.Require(canonical.CapabilityReasoning)
		default:
			return canonical.RequestEnvelope{}, fmt.Errorf("%w: unsupported Responses item type %q", core.ErrCapabilityMismatch, it.Type)
		}
	}
	return e, nil
}
func (*Adapter) Encode(e canonical.RequestEnvelope, mode compiler.LossMode) ([]byte, compiler.Report, error) {
	report := compiler.NewReport(mode)
	req := request{Model: e.Model, Stream: e.Stream, MaxOutputTokens: e.Requirements.MaximumOutputTokens, Metadata: e.Metadata}
	req.Tools = append([]byte(nil), e.Extensions["openai.responses.tools"]...)
	req.Reasoning = append([]byte(nil), e.Extensions["openai.responses.reasoning"]...)
	req.Text = append([]byte(nil), e.Extensions["openai.responses.text"]...)
	if raw := e.Extensions["openai.responses.previous_response_id"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &req.PreviousResponseID)
	}
	items := make([]inputItem, 0, len(e.Items))
	for i, it := range e.Items {
		switch it.Kind {
		case canonical.ItemMessage:
			content, err := encodeParts(it.Content)
			if err != nil {
				return nil, report, err
			}
			items = append(items, inputItem{Type: "message", Role: string(it.Role), Content: content})
		case canonical.ItemToolCall:
			if it.ToolCall != nil {
				items = append(items, inputItem{Type: "function_call", CallID: it.ToolCall.ID, Name: it.ToolCall.Name, Arguments: append([]byte(nil), it.ToolCall.Arguments...)})
			}
		case canonical.ItemToolResult:
			if it.ToolResult != nil {
				output, err := encodeParts(it.ToolResult.Content)
				if err != nil {
					return nil, report, err
				}
				items = append(items, inputItem{Type: "function_call_output", CallID: it.ToolResult.ToolCallID, Output: output})
			}
		case canonical.ItemReasoning:
			if e.IngressProtocol != canonical.ProtocolOpenAIResponses || it.Reasoning == nil || len(it.Reasoning.Opaque) == 0 {
				report.Add(fmt.Sprintf("items[%d].reasoning", i), e.IngressProtocol, canonical.ProtocolOpenAIResponses, "reasoning items can only be replayed when their provider-native opaque form is available")
				continue
			}
			items = append(items, inputItem{Type: "reasoning", Summary: append([]byte(nil), it.Reasoning.Opaque...)})
		default:
			return nil, report, fmt.Errorf("%w: item kind %s", core.ErrInvalidArgument, it.Kind)
		}
	}
	if report.HasFatalLoss() {
		return nil, report, core.ErrLossyTransformation
	}
	req.Input, _ = json.Marshal(items)
	body, err := json.Marshal(req)
	return body, report, err
}
func decodeParts(raw json.RawMessage) ([]canonical.ContentPart, bool, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, false, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []canonical.ContentPart{{Type: "text", Text: text}}, false, nil
	}
	var parts []part
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, false, fmt.Errorf("%w: invalid Responses content", core.ErrInvalidArgument)
	}
	out := make([]canonical.ContentPart, 0, len(parts))
	image := false
	for _, p := range parts {
		switch p.Type {
		case "input_text", "output_text", "text":
			out = append(out, canonical.ContentPart{Type: "text", Text: p.Text})
		case "input_image":
			out = append(out, canonical.ContentPart{Type: "image", URI: p.ImageURL})
			image = true
		case "input_file":
			out = append(out, canonical.ContentPart{Type: "file", URI: p.FileID})
		default:
			out = append(out, canonical.ContentPart{Type: p.Type})
		}
	}
	return out, image, nil
}
func encodeParts(parts []canonical.ContentPart) (json.RawMessage, error) {
	values := make([]part, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			values = append(values, part{Type: "input_text", Text: p.Text})
		case "image":
			values = append(values, part{Type: "input_image", ImageURL: p.URI})
		case "file":
			values = append(values, part{Type: "input_file", FileID: p.URI})
		default:
			return nil, fmt.Errorf("%w: content type %s", core.ErrCapabilityMismatch, p.Type)
		}
	}
	return json.Marshal(values)
}
