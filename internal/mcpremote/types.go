package mcpremote

import (
	"context"
	"log/slog"

	"github.com/your-org/ai-runtime-gateway/internal/auth/mcpscope"
	"github.com/your-org/ai-runtime-gateway/internal/auth/scopedtoken"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
)

const (
	ScopeConsultationsRead   = mcpscope.ConsultationsRead
	ScopeConsultationsAnswer = mcpscope.ConsultationsAnswer
	ScopeConsultationsAdmin  = mcpscope.ConsultationsAdmin
)

type ConsultationGetInput struct {
	ConsultationID string `json:"consultationId" jsonschema:"consultation identifier"`
}

type ConsultationGetOutput struct {
	ConsultationID string           `json:"consultationId"`
	Objective      string           `json:"objective"`
	TaskType       string           `json:"taskType,omitempty"`
	ContextPack    contextpack.Pack `json:"contextPack"`
	State          string           `json:"state"`
	MaximumCost    int64            `json:"maximumCostMinor,omitempty"`
}

type SubmitResultInput struct {
	ConsultationID   string `json:"consultationId" jsonschema:"consultation identifier"`
	StructuredResult any    `json:"structuredResult" jsonschema:"structured expert result"`
	ModelAttestation string `json:"modelAttestation" jsonschema:"provider verified or user declared model attestation"`
}

type SubmitResultOutput struct {
	ConsultationID   string `json:"consultationId"`
	ResultID         string `json:"resultId"`
	State            string `json:"state"`
	ModelAttestation string `json:"modelAttestation"`
}

type Backend interface {
	GetConsultation(context.Context, ConsultationGetInput) (ConsultationGetOutput, error)
	SubmitResult(context.Context, SubmitResultInput) (SubmitResultOutput, error)
}

type HandlerOptions struct {
	BearerToken      string
	TokenVerifier    scopedtoken.Verifier
	AllowedHosts     []string
	AllowedOrigins   []string
	MaximumBodyBytes int64
	Logger           *slog.Logger
}
