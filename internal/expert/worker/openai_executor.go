package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	"github.com/your-org/ai-runtime-gateway/internal/expert/resultcontract"
	"github.com/your-org/ai-runtime-gateway/internal/expert/routes/openaiapi"
)

type OpenAIExecutor struct {
	Client              *openaiapi.Client
	Model               string
	ReasoningMode       string
	ReasoningEffort     string
	MaximumOutputTokens int64
}

func (e OpenAIExecutor) Execute(ctx context.Context, item consultation.Consultation, pack contextpack.Pack) (resultcontract.Result, error) {
	return e.Client.Run(ctx, pack, openaiapi.RunOptions{
		Model:               e.Model,
		ReasoningMode:       e.ReasoningMode,
		ReasoningEffort:     e.ReasoningEffort,
		MaximumCostMinor:    item.MaximumCostMinor,
		MaximumOutputTokens: e.MaximumOutputTokens,
	})
}

func (e OpenAIExecutor) ModelAttestation(_ consultation.Consultation) string {
	model := strings.TrimSpace(e.Model)
	if model == "" {
		model = "unknown"
	}
	mode := strings.TrimSpace(e.ReasoningMode)
	if mode == "" {
		mode = "default"
	}
	effort := strings.TrimSpace(e.ReasoningEffort)
	if effort == "" {
		effort = "default"
	}
	return fmt.Sprintf("provider=openai;model=%s;reasoning_mode=%s;reasoning_effort=%s;attestation=configured_request", model, mode, effort)
}
