package canonical

/* llmnav/1 module
id=relaydock.protocol.canonical-envelope
role=Define the provider-neutral request vocabulary that preserves messages, tools, reasoning, capabilities, metadata, and unknown extensions.
owns=canonical request envelope|canonical item vocabulary|request structural validation
excludes=provider wire conversion|stream event transitions
search=canonical request envelope|protocol item types|preserve protocol extensions
invariant=Unknown extension payloads remain available as raw JSON for compatible conversion.
invariant=Every item kind must carry the fields required by its canonical meaning.
stability=contract
*/

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
)

type Protocol string
type ItemKind string
type Role string

const (
	ProtocolOpenAIResponses   Protocol = "openai.responses"
	ProtocolOpenAIChat        Protocol = "openai.chat_completions"
	ProtocolAnthropicMessages Protocol = "anthropic.messages"
	ProtocolGeminiGenerate    Protocol = "gemini.generate_content"
)

const (
	ItemMessage    ItemKind = "message"
	ItemToolCall   ItemKind = "tool_call"
	ItemToolResult ItemKind = "tool_result"
	ItemReasoning  ItemKind = "reasoning"
)

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type RequestEnvelope struct {
	RequestID       string
	TenantID        string
	ProjectID       string
	IngressProtocol Protocol
	Model           string
	Stream          bool
	Items           []Item
	Requirements    CapabilityRequirements
	Extensions      map[string]json.RawMessage
	Metadata        map[string]string
	RoutingPolicyID string
	PriceRevisionID string
}

type Item struct {
	ID         string
	Kind       ItemKind
	Role       Role
	Content    []ContentPart
	ToolCall   *ToolCall
	ToolResult *ToolResult
	Reasoning  *ReasoningBlock
	Extensions map[string]json.RawMessage
}

type ContentPart struct{ Type, Text, URI, MIME string }
type ToolCall struct {
	ID, Name  string
	Arguments json.RawMessage
}
type ToolResult struct {
	ToolCallID string
	Content    []ContentPart
	IsError    bool
}
type ReasoningBlock struct {
	Summary, Signature string
	Opaque             json.RawMessage
}

func (e RequestEnvelope) Validate() error {
	if e.IngressProtocol == "" {
		return fmt.Errorf("%w: ingress protocol is required", core.ErrInvalidArgument)
	}
	if len(e.Items) == 0 {
		return fmt.Errorf("%w: request contains no items", core.ErrInvalidArgument)
	}
	for i, item := range e.Items {
		if item.Kind == "" {
			return fmt.Errorf("%w: item %d has no kind", core.ErrInvalidArgument, i)
		}
		switch item.Kind {
		case ItemMessage:
			if item.Role == "" {
				return fmt.Errorf("%w: message item %d has no role", core.ErrInvalidArgument, i)
			}
			if len(item.Content) == 0 {
				return fmt.Errorf("%w: message item %d has no content", core.ErrInvalidArgument, i)
			}
		case ItemToolCall:
			if item.ToolCall == nil || strings.TrimSpace(item.ToolCall.Name) == "" {
				return fmt.Errorf("%w: tool call item %d is incomplete", core.ErrInvalidArgument, i)
			}
		case ItemToolResult:
			if item.ToolResult == nil || item.ToolResult.ToolCallID == "" {
				return fmt.Errorf("%w: tool result item %d is incomplete", core.ErrInvalidArgument, i)
			}
		case ItemReasoning:
			if item.Reasoning == nil {
				return fmt.Errorf("%w: reasoning item %d is incomplete", core.ErrInvalidArgument, i)
			}
		default:
			return fmt.Errorf("%w: unsupported item kind %q", core.ErrInvalidArgument, item.Kind)
		}
	}
	return nil
}

func (e RequestEnvelope) Text() string {
	var b strings.Builder
	for _, item := range e.Items {
		for _, part := range item.Content {
			if part.Type == "text" && part.Text != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(part.Text)
			}
		}
	}
	return b.String()
}

func (e *RequestEnvelope) Require(capability Capability) {
	for _, existing := range e.Requirements.Required {
		if existing == capability {
			return
		}
	}
	e.Requirements.Required = append(e.Requirements.Required, capability)
}
