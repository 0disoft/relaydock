package conformance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/compiler"
	"github.com/0disoft/relaydock/internal/protocol/defaults"
	"github.com/0disoft/relaydock/internal/protocol/stream"
	"github.com/0disoft/relaydock/internal/provider/httpadapter"
)

func TestOpenAIResponsesRoundTrip(t *testing.T) {
	body := []byte(`{
		"model":"gpt-test",
		"instructions":"Treat the repository contract as authoritative.",
		"stream":true,
		"max_output_tokens":4096,
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"Review the ledger."}]},
			{"type":"function_call","call_id":"call_1","name":"read_file","arguments":{"path":"ledger.go"}},
			{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_text","text":"package ledger"}]},
			{"type":"reasoning","summary":[{"type":"summary_text","text":"Inspect idempotency."}]}
		]
	}`)

	service := defaults.Compiler()
	first, err := service.Decode(context.Background(), canonical.ProtocolOpenAIResponses, body)
	if err != nil {
		t.Fatalf("decode source: %v", err)
	}
	encoded, report, err := service.Encode(context.Background(), first, canonical.ProtocolOpenAIResponses, compiler.LossModeStrict)
	if err != nil {
		t.Fatalf("encode source: %v (report=%+v)", err, report)
	}
	if len(report.Losses) != 0 {
		t.Fatalf("same-protocol round trip reported loss: %+v", report.Losses)
	}
	second, err := service.Decode(context.Background(), canonical.ProtocolOpenAIResponses, encoded.Body)
	if err != nil {
		t.Fatalf("decode encoded body: %v\n%s", err, encoded.Body)
	}

	if first.Model != second.Model || first.Stream != second.Stream {
		t.Fatalf("request metadata changed: first=%+v second=%+v", first, second)
	}
	if first.Requirements.MaximumOutputTokens != second.Requirements.MaximumOutputTokens {
		t.Fatalf("maximum output tokens changed: %d != %d", first.Requirements.MaximumOutputTokens, second.Requirements.MaximumOutputTokens)
	}
	if first.Text() != second.Text() {
		t.Fatalf("text changed: %q != %q", first.Text(), second.Text())
	}
	if len(first.Items) != len(second.Items) {
		t.Fatalf("item count changed: %d != %d", len(first.Items), len(second.Items))
	}
	for i := range first.Items {
		if first.Items[i].Kind != second.Items[i].Kind || first.Items[i].Role != second.Items[i].Role {
			t.Fatalf("item %d changed kind or role: first=%+v second=%+v", i, first.Items[i], second.Items[i])
		}
	}
}

func TestUnknownStreamEventIsPreserved(t *testing.T) {
	raw := []byte(`{"type":"response.future_metadata","checkpoint":"abc","value":7}`)
	events := httpadapter.DecodePayload("future-provider", canonical.ProtocolOpenAIResponses, raw)
	if len(events) != 1 {
		t.Fatalf("unknown event expanded unexpectedly: %+v", events)
	}
	event := events[0]
	if event.Kind != stream.EventProviderRaw {
		t.Fatalf("unknown event lost: got kind %q", event.Kind)
	}
	if !bytes.Equal(event.Delta, raw) {
		t.Fatalf("raw provider payload changed: %s", event.Delta)
	}
	var eventType string
	if err := json.Unmarshal(event.Extension["type"], &eventType); err != nil {
		t.Fatalf("decode preserved type: %v", err)
	}
	if eventType != "response.future_metadata" {
		t.Fatalf("wrong preserved type: %q", eventType)
	}

	machine := stream.NewStateMachine()
	if err := machine.Accept(event); err != nil {
		t.Fatalf("forward-compatible event rejected: %v", err)
	}
	if !machine.CanRetryTransparently() {
		t.Fatal("provider metadata incorrectly disabled transparent pre-semantic retry")
	}
}
