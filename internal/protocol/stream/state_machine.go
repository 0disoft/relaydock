package stream

/* llmnav/1 module
id=relaydock.protocol.stream-state
role=Serialize canonical stream transitions and expose whether a failed attempt remains safe for transparent retry.
owns=stream lifecycle state|semantic event commit flag|stream sequence monotonicity
excludes=wire encoding|provider retry selection
search=stream state machine|transparent retry boundary|semantic event sequence
invariant=Terminal streams reject every later event and cancellation.
invariant=Transparent retry becomes impossible after the first semantic event is accepted.
stability=contract
*/

import (
	"fmt"
	"sync"

	"github.com/0disoft/relaydock/internal/core"
)

type State string

const (
	StateCreated   State = "created"
	StateStreaming State = "streaming"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

type StateMachine struct {
	mu                     sync.Mutex
	state                  State
	firstSemanticEventSent bool
	lastSequence           int64
}

func NewStateMachine() *StateMachine { return &StateMachine{state: StateCreated} }

func (m *StateMachine) Accept(event Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateCompleted || m.state == StateFailed || m.state == StateCancelled {
		return fmt.Errorf("%w: terminal state %s", core.ErrInvalidTransition, m.state)
	}
	if event.Sequence > 0 {
		if event.Sequence <= m.lastSequence {
			return fmt.Errorf("%w: sequence %d after %d", core.ErrInvalidTransition, event.Sequence, m.lastSequence)
		}
		m.lastSequence = event.Sequence
	}
	switch event.Kind {
	case EventResponseStarted:
		if m.state != StateCreated {
			return fmt.Errorf("%w: duplicate response start", core.ErrInvalidTransition)
		}
		m.state = StateStreaming
	case EventContentDelta, EventToolCallDelta, EventReasoningDelta:
		if m.state == StateCreated {
			m.state = StateStreaming
		}
		m.firstSemanticEventSent = true
	case EventUsage:
		if event.Usage == nil {
			return fmt.Errorf("%w: usage event has no usage", core.ErrInvalidArgument)
		}
		if m.state == StateCreated {
			m.state = StateStreaming
		}
	case EventProviderRaw:
		// Unknown provider events are intentionally preserved for forward compatibility.
		// They do not count as semantic output, so a transport failure that follows may
		// still be retried before user-visible content has been emitted.
		if m.state == StateCreated {
			m.state = StateStreaming
		}
	case EventCompleted:
		m.state = StateCompleted
	case EventFailed:
		m.state = StateFailed
	default:
		return fmt.Errorf("%w: unknown stream event %q", core.ErrInvalidArgument, event.Kind)
	}
	return nil
}
func (m *StateMachine) Cancel() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateCompleted || m.state == StateFailed || m.state == StateCancelled {
		return fmt.Errorf("%w: terminal state %s", core.ErrInvalidTransition, m.state)
	}
	m.state = StateCancelled
	return nil
}
func (m *StateMachine) CanRetryTransparently() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.firstSemanticEventSent && m.state != StateCompleted && m.state != StateCancelled
}
func (m *StateMachine) State() State { m.mu.Lock(); defer m.mu.Unlock(); return m.state }
func (m *StateMachine) FirstSemanticEventSent() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.firstSemanticEventSent
}
