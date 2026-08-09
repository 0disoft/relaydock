package generate

import (
	"encoding/json"
	"fmt"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/compiler"
)

type Adapter struct{}

func New() *Adapter                           { return &Adapter{} }
func (*Adapter) Protocol() canonical.Protocol { return canonical.ProtocolGeminiGenerate }

type request struct {
	Contents          []content       `json:"contents"`
	SystemInstruction *content        `json:"systemInstruction,omitempty"`
	Tools             json.RawMessage `json:"tools,omitempty"`
	GenerationConfig  json.RawMessage `json:"generationConfig,omitempty"`
	CachedContent     string          `json:"cachedContent,omitempty"`
}
type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}
type part struct {
	Text             string            `json:"text,omitempty"`
	InlineData       *blob             `json:"inlineData,omitempty"`
	FileData         *fileData         `json:"fileData,omitempty"`
	FunctionCall     *functionCall     `json:"functionCall,omitempty"`
	FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
	ThoughtSignature string            `json:"thoughtSignature,omitempty"`
}
type blob struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}
type fileData struct {
	MIMEType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`
}
type functionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}
type functionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
	ID       string          `json:"id,omitempty"`
}

func (*Adapter) Decode(body []byte) (canonical.RequestEnvelope, error) {
	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		return canonical.RequestEnvelope{}, fmt.Errorf("%w: %v", core.ErrInvalidArgument, err)
	}
	e := canonical.RequestEnvelope{RequestID: idgen.New("req"), IngressProtocol: canonical.ProtocolGeminiGenerate, Extensions: map[string]json.RawMessage{}}
	e.Require(canonical.CapabilityTextInput)
	if len(req.Tools) > 0 {
		e.Extensions["gemini.tools"] = append([]byte(nil), req.Tools...)
		e.Require(canonical.CapabilityToolCalling)
	}
	if len(req.GenerationConfig) > 0 {
		e.Extensions["gemini.generation_config"] = append([]byte(nil), req.GenerationConfig...)
	}
	if req.CachedContent != "" {
		e.Extensions["gemini.cached_content"], _ = json.Marshal(req.CachedContent)
		e.Require(canonical.CapabilityPromptCaching)
	}
	if req.SystemInstruction != nil {
		items, err := decodeContent(*req.SystemInstruction, canonical.RoleSystem)
		if err != nil {
			return canonical.RequestEnvelope{}, err
		}
		e.Items = append(e.Items, items...)
	}
	for _, c := range req.Contents {
		role := canonical.RoleUser
		if c.Role == "model" {
			role = canonical.RoleAssistant
		}
		items, err := decodeContent(c, role)
		if err != nil {
			return canonical.RequestEnvelope{}, err
		}
		for _, it := range items {
			if len(it.Content) > 0 && it.Content[0].Type == "image" {
				e.Require(canonical.CapabilityImageInput)
			}
			if it.Kind == canonical.ItemToolCall {
				e.Require(canonical.CapabilityToolCalling)
			}
			if it.Kind == canonical.ItemReasoning {
				e.Require(canonical.CapabilityReasoning)
			}
		}
		e.Items = append(e.Items, items...)
	}
	return e, nil
}
func (*Adapter) Encode(e canonical.RequestEnvelope, mode compiler.LossMode) ([]byte, compiler.Report, error) {
	report := compiler.NewReport(mode)
	req := request{Tools: append([]byte(nil), e.Extensions["gemini.tools"]...), GenerationConfig: append([]byte(nil), e.Extensions["gemini.generation_config"]...)}
	if raw := e.Extensions["gemini.cached_content"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &req.CachedContent)
	}
	for i, it := range e.Items {
		switch it.Kind {
		case canonical.ItemMessage:
			parts, err := encodeParts(it.Content)
			if err != nil {
				return nil, report, err
			}
			c := content{Role: "user", Parts: parts}
			if it.Role == canonical.RoleAssistant {
				c.Role = "model"
			}
			if it.Role == canonical.RoleSystem {
				req.SystemInstruction = &content{Parts: parts}
			} else {
				req.Contents = append(req.Contents, c)
			}
		case canonical.ItemToolCall:
			if it.ToolCall != nil {
				req.Contents = append(req.Contents, content{Role: "model", Parts: []part{{FunctionCall: &functionCall{Name: it.ToolCall.Name, Args: append([]byte(nil), it.ToolCall.Arguments...)}}}})
			}
		case canonical.ItemToolResult:
			if it.ToolResult != nil {
				resp, _ := json.Marshal(map[string]any{"content": it.ToolResult.Content, "isError": it.ToolResult.IsError})
				req.Contents = append(req.Contents, content{Role: "user", Parts: []part{{FunctionResponse: &functionResponse{ID: it.ToolResult.ToolCallID, Response: resp}}}})
			}
		case canonical.ItemReasoning:
			if e.IngressProtocol == canonical.ProtocolGeminiGenerate && it.Reasoning != nil {
				req.Contents = append(req.Contents, content{Role: "model", Parts: []part{{Text: it.Reasoning.Summary, Thought: true, ThoughtSignature: it.Reasoning.Signature}}})
			} else {
				report.Add(fmt.Sprintf("items[%d].reasoning", i), e.IngressProtocol, canonical.ProtocolGeminiGenerate, "Gemini thought signatures cannot be synthesized across providers")
			}
		}
	}
	if report.HasFatalLoss() {
		return nil, report, core.ErrLossyTransformation
	}
	body, err := json.Marshal(req)
	return body, report, err
}
func decodeContent(c content, role canonical.Role) ([]canonical.Item, error) {
	out := make([]canonical.Item, 0, len(c.Parts))
	for _, p := range c.Parts {
		switch {
		case p.FunctionCall != nil:
			out = append(out, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemToolCall, Role: canonical.RoleAssistant, ToolCall: &canonical.ToolCall{Name: p.FunctionCall.Name, Arguments: append([]byte(nil), p.FunctionCall.Args...)}})
		case p.FunctionResponse != nil:
			out = append(out, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemToolResult, Role: canonical.RoleTool, ToolResult: &canonical.ToolResult{ToolCallID: p.FunctionResponse.ID, Content: []canonical.ContentPart{{Type: "json", Text: string(p.FunctionResponse.Response)}}}})
		case p.Thought:
			out = append(out, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemReasoning, Role: canonical.RoleAssistant, Reasoning: &canonical.ReasoningBlock{Summary: p.Text, Signature: p.ThoughtSignature}})
		case p.InlineData != nil:
			out = append(out, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemMessage, Role: role, Content: []canonical.ContentPart{{Type: "image", URI: "data:" + p.InlineData.MIMEType + ";base64," + p.InlineData.Data, MIME: p.InlineData.MIMEType}}})
		case p.FileData != nil:
			out = append(out, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemMessage, Role: role, Content: []canonical.ContentPart{{Type: "image", URI: p.FileData.FileURI, MIME: p.FileData.MIMEType}}})
		default:
			out = append(out, canonical.Item{ID: idgen.New("item"), Kind: canonical.ItemMessage, Role: role, Content: []canonical.ContentPart{{Type: "text", Text: p.Text}}})
		}
	}
	return out, nil
}
func encodeParts(parts []canonical.ContentPart) ([]part, error) {
	out := make([]part, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			out = append(out, part{Text: p.Text})
		case "image":
			out = append(out, part{FileData: &fileData{MIMEType: p.MIME, FileURI: p.URI}})
		case "json":
			out = append(out, part{Text: p.Text})
		default:
			return nil, fmt.Errorf("%w: content type %s", core.ErrCapabilityMismatch, p.Type)
		}
	}
	return out, nil
}
