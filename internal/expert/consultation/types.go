package consultation

import "time"

type State string
type Route string

const (
	StateCreated         State = "created"
	StateContextPending  State = "context_pending"
	StateApprovalPending State = "approval_pending"
	StateQueued          State = "queued"
	StateRunning         State = "running"
	StateResultPending   State = "result_pending"
	StateCompleted       State = "completed"
	StateFailed          State = "failed"
	StateCancelled       State = "cancelled"
	StateExpired         State = "expired"
)
const (
	RouteOpenAIAPIPro Route = "openai_api_pro"
	RouteWebHandoff   Route = "chatgpt_web_handoff"
)

type Consultation struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenantId,omitempty"`
	ProjectID        string    `json:"projectId,omitempty"`
	Objective        string    `json:"objective"`
	TaskType         string    `json:"taskType,omitempty"`
	Route            Route     `json:"route"`
	State            State     `json:"state"`
	ContextPackID    string    `json:"contextPackId,omitempty"`
	MaximumCostMinor int64     `json:"maximumCostMinor,omitempty"`
	ResultID         string    `json:"resultId,omitempty"`
	FailureReason    string    `json:"failureReason,omitempty"`
	AttemptCount     int       `json:"attemptCount,omitempty"`
	AvailableAt      time.Time `json:"availableAt,omitempty"`
	LockedBy         string    `json:"lockedBy,omitempty"`
	LockExpiresAt    time.Time `json:"lockExpiresAt,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	ExpiresAt        time.Time `json:"expiresAt"`
}

type CreateCommand struct {
	TenantID         string        `json:"tenantId,omitempty"`
	ProjectID        string        `json:"projectId,omitempty"`
	Objective        string        `json:"objective"`
	TaskType         string        `json:"taskType,omitempty"`
	Route            Route         `json:"route,omitempty"`
	ContextPackID    string        `json:"contextPackId,omitempty"`
	MaximumCostMinor int64         `json:"maximumCostMinor,omitempty"`
	IdempotencyKey   string        `json:"idempotencyKey,omitempty"`
	TTL              time.Duration `json:"-"`
}

func (c Consultation) Terminal() bool {
	return c.State == StateCompleted || c.State == StateFailed || c.State == StateCancelled || c.State == StateExpired
}
