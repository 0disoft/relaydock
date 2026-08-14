package consultation

/* llmnav/1 module
id=relaydock.expert.lifecycle
role=Define allowed consultation state transitions from context request through approval, execution, result, and terminal outcomes.
owns=consultation state graph|consultation lifecycle events
excludes=state persistence|worker lease claims
search=consultation state machine|approve expert consultation|complete consultation lifecycle
invariant=Only declared state-event pairs can advance a consultation.
stability=contract
*/

import (
	"fmt"
	"github.com/0disoft/relaydock/internal/core"
)

type Event string

const (
	EventContextRequested Event = "context_requested"
	EventContextAttached  Event = "context_attached"
	EventApproved         Event = "approved"
	EventStarted          Event = "started"
	EventResultReceived   Event = "result_received"
	EventCompleted        Event = "completed"
	EventFailed           Event = "failed"
	EventCancelled        Event = "cancelled"
	EventExpired          Event = "expired"
)

var transitions = map[State]map[Event]State{
	StateCreated:         {EventContextRequested: StateContextPending, EventContextAttached: StateApprovalPending, EventFailed: StateFailed, EventCancelled: StateCancelled, EventExpired: StateExpired},
	StateContextPending:  {EventContextAttached: StateApprovalPending, EventFailed: StateFailed, EventCancelled: StateCancelled, EventExpired: StateExpired},
	StateApprovalPending: {EventApproved: StateQueued, EventFailed: StateFailed, EventCancelled: StateCancelled, EventExpired: StateExpired},
	StateQueued:          {EventStarted: StateRunning, EventFailed: StateFailed, EventCancelled: StateCancelled, EventExpired: StateExpired},
	StateRunning:         {EventResultReceived: StateResultPending, EventFailed: StateFailed, EventCancelled: StateCancelled, EventExpired: StateExpired},
	StateResultPending:   {EventCompleted: StateCompleted, EventFailed: StateFailed, EventCancelled: StateCancelled, EventExpired: StateExpired},
}

func Transition(current State, event Event) (State, error) {
	if next, ok := transitions[current][event]; ok {
		return next, nil
	}
	return "", fmt.Errorf("%w: %s + %s", core.ErrInvalidTransition, current, event)
}
