package expert_test

import (
	"errors"
	"testing"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/expert/consultation"
	expertpolicy "github.com/0disoft/relaydock/internal/expert/policy"
)

func TestConsultationTransitionTable(t *testing.T) {
	allowed := []struct {
		from  consultation.State
		event consultation.Event
		to    consultation.State
	}{
		{consultation.StateCreated, consultation.EventContextRequested, consultation.StateContextPending},
		{consultation.StateCreated, consultation.EventContextAttached, consultation.StateApprovalPending},
		{consultation.StateContextPending, consultation.EventContextAttached, consultation.StateApprovalPending},
		{consultation.StateApprovalPending, consultation.EventApproved, consultation.StateQueued},
		{consultation.StateQueued, consultation.EventStarted, consultation.StateRunning},
		{consultation.StateRunning, consultation.EventResultReceived, consultation.StateResultPending},
		{consultation.StateResultPending, consultation.EventCompleted, consultation.StateCompleted},
	}
	for _, tc := range allowed {
		got, err := consultation.Transition(tc.from, tc.event)
		if err != nil {
			t.Fatalf("allowed transition %s + %s failed: %v", tc.from, tc.event, err)
		}
		if got != tc.to {
			t.Fatalf("transition %s + %s = %s, want %s", tc.from, tc.event, got, tc.to)
		}
	}

	terminal := []consultation.State{
		consultation.StateCompleted,
		consultation.StateFailed,
		consultation.StateCancelled,
		consultation.StateExpired,
	}
	for _, state := range terminal {
		if _, err := consultation.Transition(state, consultation.EventStarted); !errors.Is(err, core.ErrInvalidTransition) {
			t.Fatalf("terminal state %s accepted work: %v", state, err)
		}
	}
	if _, err := consultation.Transition(consultation.StateApprovalPending, consultation.EventCompleted); !errors.Is(err, core.ErrInvalidTransition) {
		t.Fatalf("approval-pending consultation skipped execution states: %v", err)
	}
}

func TestDelegationDepthStopsRecursiveExpertCalls(t *testing.T) {
	guard := expertpolicy.NewDelegationGuard(1)
	for _, depth := range []int{0, 1} {
		if err := guard.Check(depth); err != nil {
			t.Fatalf("depth %d should be allowed: %v", depth, err)
		}
	}
	if err := guard.Check(2); !errors.Is(err, core.ErrForbidden) {
		t.Fatalf("recursive expert call was not blocked: %v", err)
	}
	if err := guard.Check(-1); !errors.Is(err, core.ErrInvalidArgument) {
		t.Fatalf("negative delegation depth was not rejected: %v", err)
	}
}
