package httpgateway

/* llmnav/1 module
id=relaydock.transport.live-stream
role=Commit provider metadata and translate canonical stream events into OpenAI-compatible live HTTP output without reopening retry eligibility.
owns=HTTP stream commit boundary|SSE response encoding|stream terminal emission
excludes=provider execution|canonical stream validation
search=write live SSE response|commit provider stream|encode OpenAI stream event
invariant=Headers and the streaming status line are committed only once.
invariant=No semantic event is emitted before the upstream route commit hook succeeds.
stability=contract
*/

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/stream"
	"github.com/0disoft/relaydock/internal/provider"
)

// StreamMetadata is known at the moment the runtime commits to an upstream
// route. Before this callback fires, an upstream failure can still be retried
// without exposing a partial response to the client.
type StreamMetadata struct {
	RequestID     string
	Model         string
	Provider      string
	UpstreamModel string
	CreatedAt     time.Time
}

type StreamHooks struct {
	OnCommit func(context.Context, StreamMetadata) error
	OnEvent  func(context.Context, stream.Event) error
}

// StreamingProcessor is an optional extension implemented by processors that
// can forward canonical events as they arrive. The ordinary Processor contract
// remains available for non-streaming callers and simple test doubles.
type StreamingProcessor interface {
	Stream(context.Context, canonical.RequestEnvelope, StreamHooks) (Completion, error)
}

type liveStreamWriter struct {
	writer       http.ResponseWriter
	flusher      http.Flusher
	protocol     canonical.Protocol
	requestID    string
	model        string
	createdAt    time.Time
	committed    bool
	started      bool
	terminal     bool
	contentOpen  bool
	chatRoleSent bool
	text         bytes.Buffer
	usage        stream.Usage
}

func newLiveStreamWriter(writer http.ResponseWriter, protocol canonical.Protocol, requestID, model string) (*liveStreamWriter, error) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("%w: response writer does not support streaming", core.ErrInvalidConfiguration)
	}
	return &liveStreamWriter{
		writer:    writer,
		flusher:   flusher,
		protocol:  protocol,
		requestID: requestID,
		model:     model,
		createdAt: time.Now().UTC(),
	}, nil
}

func (s *liveStreamWriter) Commit(metadata StreamMetadata) error {
	if s.committed {
		return nil
	}
	if metadata.RequestID != "" {
		s.requestID = metadata.RequestID
	}
	if metadata.Model != "" {
		s.model = metadata.Model
	}
	if !metadata.CreatedAt.IsZero() {
		s.createdAt = metadata.CreatedAt.UTC()
	}
	if metadata.Provider != "" {
		s.writer.Header().Set("X-AI-Runtime-Provider", metadata.Provider)
	}
	if metadata.UpstreamModel != "" {
		s.writer.Header().Set("X-AI-Runtime-Upstream-Model", metadata.UpstreamModel)
	}
	// Attempt count is only final after the runtime exits. Declare it as a
	// trailer before the status line is written.
	s.writer.Header().Add("Trailer", "X-AI-Runtime-Attempts")
	s.writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	s.writer.Header().Set("Cache-Control", "no-cache, no-transform")
	s.writer.Header().Set("Connection", "keep-alive")
	s.writer.WriteHeader(http.StatusOK)
	s.committed = true
	return nil
}

func (s *liveStreamWriter) Event(event stream.Event) error {
	if !s.committed {
		return fmt.Errorf("%w: stream event before route commitment", core.ErrInvalidTransition)
	}
	if s.terminal {
		return fmt.Errorf("%w: stream event after terminal event", core.ErrInvalidTransition)
	}
	if event.Usage != nil {
		s.usage.Add(*event.Usage)
	}
	switch event.Kind {
	case stream.EventResponseStarted:
		return s.ensureStarted()
	case stream.EventContentDelta:
		if err := s.ensureStarted(); err != nil {
			return err
		}
		_, _ = s.text.Write(event.Delta)
		return s.writeContentDelta(string(event.Delta))
	case stream.EventToolCallDelta, stream.EventReasoningDelta:
		// The current canonical event does not yet carry enough cross-provider
		// metadata to reconstruct a standards-compliant tool/reasoning block.
		// Fail explicitly instead of silently converting it to visible text.
		return fmt.Errorf("%w: live compatibility egress cannot represent %s", core.ErrLossyTransformation, event.Kind)
	case stream.EventUsage, stream.EventProviderRaw:
		return nil
	case stream.EventCompleted:
		if err := s.ensureStarted(); err != nil {
			return err
		}
		return s.complete()
	case stream.EventFailed:
		return s.Fail(fmt.Errorf("provider stream failed: %s", boundedMessage(string(event.Delta))))
	default:
		return fmt.Errorf("%w: stream event %s", core.ErrInvalidArgument, event.Kind)
	}
}

func (s *liveStreamWriter) ensureStarted() error {
	if s.started {
		return nil
	}
	s.started = true
	responseID := s.responseID()
	switch s.protocol {
	case canonical.ProtocolOpenAIResponses:
		created := map[string]any{
			"id": responseID, "object": "response", "created_at": s.createdAt.Unix(),
			"status": "in_progress", "model": s.model, "output": []any{},
		}
		if err := s.send("response.created", map[string]any{"type": "response.created", "response": created}); err != nil {
			return err
		}
		if err := s.send("response.in_progress", map[string]any{"type": "response.in_progress", "response": created}); err != nil {
			return err
		}
		itemID := "msg_" + responseID
		message := map[string]any{"id": itemID, "type": "message", "status": "in_progress", "role": "assistant", "content": []any{}}
		if err := s.send("response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": 0, "item": message}); err != nil {
			return err
		}
		part := map[string]any{"type": "output_text", "text": "", "annotations": []any{}}
		s.contentOpen = true
		return s.send("response.content_part.added", map[string]any{"type": "response.content_part.added", "item_id": itemID, "output_index": 0, "content_index": 0, "part": part})
	case canonical.ProtocolOpenAIChat:
		// The role is emitted with the first chunk, even when the model returns
		// an empty response.
		s.chatRoleSent = true
		return s.send("", s.chatChunk(map[string]any{"role": "assistant"}, nil, false))
	case canonical.ProtocolAnthropicMessages:
		message := map[string]any{
			"id": responseID, "type": "message", "role": "assistant", "model": s.model,
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": s.usage.InputTokens, "output_tokens": 0},
		}
		if err := s.send("message_start", map[string]any{"type": "message_start", "message": message}); err != nil {
			return err
		}
		s.contentOpen = true
		return s.send("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
	case canonical.ProtocolGeminiGenerate:
		return nil
	default:
		return fmt.Errorf("%w: stream protocol %s", core.ErrInvalidArgument, s.protocol)
	}
}

func (s *liveStreamWriter) writeContentDelta(delta string) error {
	if delta == "" {
		return nil
	}
	responseID := s.responseID()
	switch s.protocol {
	case canonical.ProtocolOpenAIResponses:
		return s.send("response.output_text.delta", map[string]any{
			"type": "response.output_text.delta", "item_id": "msg_" + responseID,
			"output_index": 0, "content_index": 0, "delta": delta,
		})
	case canonical.ProtocolOpenAIChat:
		return s.send("", s.chatChunk(map[string]any{"content": delta}, nil, false))
	case canonical.ProtocolAnthropicMessages:
		return s.send("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": delta}})
	case canonical.ProtocolGeminiGenerate:
		return s.send("", map[string]any{
			"candidates": []any{map[string]any{
				"content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": delta}}},
				"index":   0,
			}},
			"modelVersion": s.model, "responseId": responseID,
		})
	default:
		return fmt.Errorf("%w: stream protocol %s", core.ErrInvalidArgument, s.protocol)
	}
}

func (s *liveStreamWriter) complete() error {
	if s.terminal {
		return nil
	}
	responseID := s.responseID()
	text := s.text.String()
	var err error
	switch s.protocol {
	case canonical.ProtocolOpenAIResponses:
		itemID := "msg_" + responseID
		donePart := map[string]any{"type": "output_text", "text": text, "annotations": []any{}}
		if s.contentOpen {
			if err = s.send("response.output_text.done", map[string]any{"type": "response.output_text.done", "item_id": itemID, "output_index": 0, "content_index": 0, "text": text}); err != nil {
				break
			}
			if err = s.send("response.content_part.done", map[string]any{"type": "response.content_part.done", "item_id": itemID, "output_index": 0, "content_index": 0, "part": donePart}); err != nil {
				break
			}
		}
		message := map[string]any{"id": itemID, "type": "message", "status": "completed", "role": "assistant", "content": []any{donePart}}
		if err = s.send("response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": 0, "item": message}); err != nil {
			break
		}
		completion := Completion{ID: responseID, Model: s.model, Text: text, FinishReason: "stop", InputTokens: s.usage.InputTokens, OutputTokens: s.usage.OutputTokens, CreatedAt: s.createdAt}
		err = s.send("response.completed", map[string]any{"type": "response.completed", "response": openAIResponse(completion)})
	case canonical.ProtocolOpenAIChat:
		err = s.send("", s.chatChunk(map[string]any{}, "stop", true))
		if err == nil {
			_, err = fmt.Fprint(s.writer, "data: [DONE]\n\n")
			if err == nil {
				s.flusher.Flush()
			}
		}
	case canonical.ProtocolAnthropicMessages:
		if s.contentOpen {
			if err = s.send("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0}); err != nil {
				break
			}
		}
		if err = s.send("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": map[string]any{"output_tokens": s.usage.OutputTokens}}); err != nil {
			break
		}
		err = s.send("message_stop", map[string]any{"type": "message_stop"})
	case canonical.ProtocolGeminiGenerate:
		err = s.send("", map[string]any{
			"candidates": []any{map[string]any{
				"content":      map[string]any{"role": "model", "parts": []any{map[string]any{"text": ""}}},
				"finishReason": "STOP", "index": 0,
			}},
			"usageMetadata": map[string]any{
				"promptTokenCount": s.usage.InputTokens, "candidatesTokenCount": s.usage.OutputTokens,
				"totalTokenCount": s.usage.InputTokens + s.usage.OutputTokens,
			},
			"modelVersion": s.model, "responseId": responseID,
		})
	default:
		err = fmt.Errorf("%w: stream protocol %s", core.ErrInvalidArgument, s.protocol)
	}
	if err == nil {
		s.terminal = true
	}
	return err
}

func (s *liveStreamWriter) Fail(cause error) error {
	if s.terminal {
		return nil
	}
	if !s.committed {
		return cause
	}
	message := boundedMessage(cause.Error())
	code := provider.ErrorCode(cause)
	if code == "" {
		code = "upstream_error"
	}
	var err error
	switch s.protocol {
	case canonical.ProtocolOpenAIResponses:
		failure := map[string]any{
			"id": s.responseID(), "object": "response", "created_at": s.createdAt.Unix(),
			"status": "failed", "model": s.model, "output": []any{},
			"error": map[string]any{"code": code, "message": message},
		}
		err = s.send("response.failed", map[string]any{"type": "response.failed", "response": failure})
	case canonical.ProtocolOpenAIChat:
		err = s.send("error", map[string]any{"error": map[string]any{"type": code, "message": message}})
		if err == nil {
			_, err = fmt.Fprint(s.writer, "data: [DONE]\n\n")
			if err == nil {
				s.flusher.Flush()
			}
		}
	case canonical.ProtocolAnthropicMessages:
		err = s.send("error", map[string]any{"type": "error", "error": map[string]any{"type": code, "message": message}})
	case canonical.ProtocolGeminiGenerate:
		err = s.send("", map[string]any{"error": map[string]any{"code": http.StatusBadGateway, "message": message, "status": "UNAVAILABLE"}})
	default:
		err = cause
	}
	if err == nil {
		s.terminal = true
	}
	return err
}

func (s *liveStreamWriter) SetAttempts(attempts int) {
	if attempts > 0 {
		s.writer.Header().Set("X-AI-Runtime-Attempts", fmt.Sprintf("%d", attempts))
	}
}

func (s *liveStreamWriter) send(eventName string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if eventName != "" {
		if _, err = fmt.Fprintf(s.writer, "event: %s\n", eventName); err != nil {
			return err
		}
	}
	if _, err = fmt.Fprintf(s.writer, "data: %s\n\n", payload); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func (s *liveStreamWriter) chatChunk(delta map[string]any, finishReason any, includeUsage bool) map[string]any {
	chunk := map[string]any{
		"id": s.responseID(), "object": "chat.completion.chunk", "created": s.createdAt.Unix(), "model": s.model,
		"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finishReason}},
	}
	if includeUsage {
		chunk["usage"] = map[string]any{
			"prompt_tokens": s.usage.InputTokens, "completion_tokens": s.usage.OutputTokens,
			"total_tokens": s.usage.InputTokens + s.usage.OutputTokens,
		}
	}
	return chunk
}

func (s *liveStreamWriter) responseID() string {
	if strings.TrimSpace(s.requestID) == "" {
		return "resp_unknown"
	}
	return s.requestID
}

func boundedMessage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "upstream stream failed"
	}
	const maximum = 1024
	if len(value) > maximum {
		return value[:maximum] + "…"
	}
	return value
}
