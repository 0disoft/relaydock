package runtime

import (
	"context"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/protocol/stream"
)

const (
	RequestStateRunning   = "running"
	RequestStateCompleted = "completed"
	RequestStateFailed    = "failed"
	RequestStateCancelled = "cancelled"

	AttemptStateRunning   = "running"
	AttemptStateCompleted = "completed"
	AttemptStateFailed    = "failed"
	AttemptStateCancelled = "cancelled"
)

// JournalRequest describes the immutable identity and policy revision attached
// to one gateway invocation. ID is an internal gateway ID; ClientRequestID is
// the caller-provided request ID and is never used as a database primary key.
type JournalRequest struct {
	ID              string    `json:"id"`
	ClientRequestID string    `json:"clientRequestId,omitempty"`
	TenantID        string    `json:"tenantId,omitempty"`
	ProjectID       string    `json:"projectId,omitempty"`
	VirtualKeyID    string    `json:"virtualKeyId,omitempty"`
	IngressProtocol string    `json:"ingressProtocol"`
	VirtualModel    string    `json:"virtualModel"`
	PriceRevisionID string    `json:"priceRevisionId"`
	AuthorizationID string    `json:"authorizationId,omitempty"`
	StartedAt       time.Time `json:"startedAt"`
}

type JournalAttempt struct {
	ID            string    `json:"id"`
	RequestID     string    `json:"requestId"`
	Number        int       `json:"number"`
	Provider      string    `json:"provider"`
	AccountID     string    `json:"accountId,omitempty"`
	UpstreamModel string    `json:"upstreamModel"`
	Protocol      string    `json:"protocol,omitempty"`
	StartedAt     time.Time `json:"startedAt"`
}

type JournalAttemptResult struct {
	ID          string       `json:"id"`
	RequestID   string       `json:"requestId"`
	State       string       `json:"state"`
	Committed   bool         `json:"committed"`
	Usage       stream.Usage `json:"usage"`
	ErrorCode   string       `json:"errorCode,omitempty"`
	Error       string       `json:"error,omitempty"`
	CompletedAt time.Time    `json:"completedAt"`
}

type JournalRequestResult struct {
	ID          string       `json:"id"`
	State       string       `json:"state"`
	Usage       stream.Usage `json:"usage"`
	Attempts    int          `json:"attempts"`
	ErrorCode   string       `json:"errorCode,omitempty"`
	Error       string       `json:"error,omitempty"`
	CompletedAt time.Time    `json:"completedAt"`
}

const RuntimeEventSchemaVersion = 1

type AttemptFinishedEvent struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Request       JournalRequest       `json:"request"`
	Attempt       JournalAttempt       `json:"attempt"`
	Result        JournalAttemptResult `json:"result"`
}

type RequestFinishedEvent struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Request       JournalRequest       `json:"request"`
	Result        JournalRequestResult `json:"result"`
}

// Journal persists request lifecycle records. Implementations must make each
// method idempotent for an identical payload and reject conflicting rewrites.
// FinishAttempt and FinishRequest should write their outbox event in the same
// transaction as the state change.
type Journal interface {
	BeginRequest(context.Context, JournalRequest) error
	MarkCommitted(context.Context, string, time.Time) error
	BeginAttempt(context.Context, JournalAttempt) error
	FinishAttempt(context.Context, JournalAttemptResult) error
	FinishRequest(context.Context, JournalRequestResult) error
}
