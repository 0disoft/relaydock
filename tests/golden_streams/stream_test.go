package goldenstreams_test

import (
	"testing"

	"github.com/0disoft/relaydock/internal/protocol/stream"
)

func TestParallelToolCallDeltas(t *testing.T) {
	machine := stream.NewStateMachine()
	events := []stream.Event{
		{Sequence: 1, Kind: stream.EventResponseStarted},
		{Sequence: 2, Kind: stream.EventToolCallDelta, ItemID: "call_a", Delta: []byte(`{"path":"`)},
		{Sequence: 3, Kind: stream.EventToolCallDelta, ItemID: "call_b", Delta: []byte(`{"query":"`)},
		{Sequence: 4, Kind: stream.EventToolCallDelta, ItemID: "call_a", Delta: []byte(`ledger.go"}`)},
		{Sequence: 5, Kind: stream.EventToolCallDelta, ItemID: "call_b", Delta: []byte(`idempotency"}`)},
		{Sequence: 6, Kind: stream.EventCompleted},
	}
	assembled := map[string][]byte{}
	for _, event := range events {
		if err := machine.Accept(event); err != nil {
			t.Fatalf("accept %+v: %v", event, err)
		}
		if event.Kind == stream.EventToolCallDelta {
			assembled[event.ItemID] = append(assembled[event.ItemID], event.Delta...)
		}
	}
	if got := string(assembled["call_a"]); got != `{"path":"ledger.go"}` {
		t.Fatalf("call_a fragments interleaved incorrectly: %q", got)
	}
	if got := string(assembled["call_b"]); got != `{"query":"idempotency"}` {
		t.Fatalf("call_b fragments interleaved incorrectly: %q", got)
	}
	if machine.State() != stream.StateCompleted || !machine.FirstSemanticEventSent() {
		t.Fatalf("unexpected final stream state: state=%s semantic=%v", machine.State(), machine.FirstSemanticEventSent())
	}
}

func TestRetryForbiddenAfterSemanticEvent(t *testing.T) {
	machine := stream.NewStateMachine()
	if err := machine.Accept(stream.Event{Kind: stream.EventResponseStarted}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !machine.CanRetryTransparently() {
		t.Fatal("response-start metadata should not disable retry")
	}
	if err := machine.Accept(stream.Event{Kind: stream.EventContentDelta, Delta: []byte("partial answer")}); err != nil {
		t.Fatalf("content: %v", err)
	}
	if machine.CanRetryTransparently() {
		t.Fatal("transparent retry remained enabled after user-visible content")
	}
}
