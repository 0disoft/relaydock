package httpgateway

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/transport/apiutil"
)

func writeCompletion(w http.ResponseWriter, protocol canonical.Protocol, completion Completion) {
	switch protocol {
	case canonical.ProtocolOpenAIResponses:
		apiutil.WriteJSON(w, http.StatusOK, openAIResponse(completion))
	case canonical.ProtocolOpenAIChat:
		apiutil.WriteJSON(w, http.StatusOK, openAIChat(completion))
	case canonical.ProtocolAnthropicMessages:
		apiutil.WriteJSON(w, http.StatusOK, anthropicMessage(completion))
	case canonical.ProtocolGeminiGenerate:
		apiutil.WriteJSON(w, http.StatusOK, geminiResponse(completion))
	default:
		apiutil.WriteError(w, fmt.Errorf("%w: response protocol %s", core.ErrInvalidArgument, protocol))
	}
}

func writeStream(w http.ResponseWriter, protocol canonical.Protocol, completion Completion) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		apiutil.WriteError(w, fmt.Errorf("streaming unsupported by response writer"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	send := func(event string, value any) {
		payload, _ := json.Marshal(value)
		if event != "" {
			_, _ = fmt.Fprintf(w, "event: %s\n", event)
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
	}

	switch protocol {
	case canonical.ProtocolOpenAIResponses:
		itemID := "msg_" + completion.ID
		created := openAIResponse(completion)
		created["status"] = "in_progress"
		created["output"] = []any{}
		delete(created, "usage")
		message := map[string]any{
			"id": itemID, "type": "message", "status": "in_progress", "role": "assistant", "content": []any{},
		}
		part := map[string]any{"type": "output_text", "text": "", "annotations": []any{}}
		send("response.created", map[string]any{"type": "response.created", "response": created})
		send("response.in_progress", map[string]any{"type": "response.in_progress", "response": created})
		send("response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": 0, "item": message})
		send("response.content_part.added", map[string]any{"type": "response.content_part.added", "item_id": itemID, "output_index": 0, "content_index": 0, "part": part})
		send("response.output_text.delta", map[string]any{"type": "response.output_text.delta", "item_id": itemID, "output_index": 0, "content_index": 0, "delta": completion.Text})
		send("response.output_text.done", map[string]any{"type": "response.output_text.done", "item_id": itemID, "output_index": 0, "content_index": 0, "text": completion.Text})
		donePart := map[string]any{"type": "output_text", "text": completion.Text, "annotations": []any{}}
		send("response.content_part.done", map[string]any{"type": "response.content_part.done", "item_id": itemID, "output_index": 0, "content_index": 0, "part": donePart})
		doneMessage := map[string]any{
			"id": itemID, "type": "message", "status": "completed", "role": "assistant", "content": []any{donePart},
		}
		send("response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": 0, "item": doneMessage})
		send("response.completed", map[string]any{"type": "response.completed", "response": openAIResponse(completion)})
	case canonical.ProtocolOpenAIChat:
		send("", map[string]any{"id": completion.ID, "object": "chat.completion.chunk", "created": completion.CreatedAt.Unix(), "model": completion.Model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": completion.Text}, "finish_reason": nil}}})
		send("", map[string]any{"id": completion.ID, "object": "chat.completion.chunk", "created": completion.CreatedAt.Unix(), "model": completion.Model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": completion.FinishReason}}, "usage": usage(completion)})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	case canonical.ProtocolAnthropicMessages:
		startMessage := map[string]any{
			"id": completion.ID, "type": "message", "role": "assistant", "model": completion.Model,
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": completion.InputTokens, "output_tokens": 0},
		}
		send("message_start", map[string]any{"type": "message_start", "message": startMessage})
		send("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		send("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": completion.Text}})
		send("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
		send("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": map[string]any{"output_tokens": completion.OutputTokens}})
		send("message_stop", map[string]any{"type": "message_stop"})
	case canonical.ProtocolGeminiGenerate:
		send("", geminiResponse(completion))
	}
}

func openAIResponse(c Completion) map[string]any {
	return map[string]any{
		"id": c.ID, "object": "response", "created_at": c.CreatedAt.Unix(), "status": "completed", "model": c.Model,
		"output": []any{map[string]any{"id": "msg_" + c.ID, "type": "message", "status": "completed", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": c.Text, "annotations": []any{}}}}},
		"usage":  map[string]any{"input_tokens": c.InputTokens, "output_tokens": c.OutputTokens, "total_tokens": c.InputTokens + c.OutputTokens},
	}
}

func openAIChat(c Completion) map[string]any {
	return map[string]any{
		"id": c.ID, "object": "chat.completion", "created": c.CreatedAt.Unix(), "model": c.Model,
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": c.Text}, "finish_reason": c.FinishReason}},
		"usage":   usage(c),
	}
}

func anthropicMessage(c Completion) map[string]any {
	return map[string]any{
		"id": c.ID, "type": "message", "role": "assistant", "model": c.Model,
		"content":     []any{map[string]any{"type": "text", "text": c.Text}},
		"stop_reason": "end_turn", "stop_sequence": nil,
		"usage": map[string]any{"input_tokens": c.InputTokens, "output_tokens": c.OutputTokens},
	}
}

func geminiResponse(c Completion) map[string]any {
	return map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"role": "model", "parts": []any{map[string]any{"text": c.Text}}},
			"finishReason": "STOP", "index": 0,
		}},
		"usageMetadata": map[string]any{"promptTokenCount": c.InputTokens, "candidatesTokenCount": c.OutputTokens, "totalTokenCount": c.InputTokens + c.OutputTokens},
		"modelVersion":  c.Model,
		"responseId":    c.ID,
	}
}

func usage(c Completion) map[string]any {
	return map[string]any{"prompt_tokens": c.InputTokens, "completion_tokens": c.OutputTokens, "total_tokens": c.InputTokens + c.OutputTokens}
}

func estimateTokens(text string) int64 {
	if text == "" {
		return 0
	}
	return int64((len([]byte(text)) + 3) / 4)
}
