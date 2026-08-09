package httpgateway

import (
	"context"
	"fmt"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/idgen"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/compiler"
)

type Completion struct {
	ID            string
	Model         string
	UpstreamModel string
	Provider      string
	Text          string
	FinishReason  string
	InputTokens   int64
	OutputTokens  int64
	CreatedAt     time.Time
	Attempts      int
}

type Processor interface {
	Complete(context.Context, canonical.RequestEnvelope) (Completion, error)
}

type ModelSource interface {
	Models() []string
}

type HandlerOptions struct {
	Compiler         *compiler.Service
	Processor        Processor
	MaximumBodyBytes int64
	Models           []string
	ModelSource      ModelSource
	Ready            func() bool
	RuntimeStatus    func(context.Context) (map[string]any, error)
	Mode             string
}

type EchoProcessor struct{}

func (EchoProcessor) Complete(ctx context.Context, request canonical.RequestEnvelope) (Completion, error) {
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	if err := request.Validate(); err != nil {
		return Completion{}, err
	}
	inputTokens := estimateTokens(request.Text())
	model := request.Model
	if model == "" {
		model = "local/echo"
	}
	text := fmt.Sprintf("Local gateway accepted %d canonical items for %s. Estimated input: %d tokens.", len(request.Items), model, inputTokens)
	return Completion{
		ID:            idgen.New("resp"),
		Model:         model,
		UpstreamModel: model,
		Provider:      "local-echo",
		Text:          text,
		FinishReason:  "stop",
		InputTokens:   inputTokens,
		OutputTokens:  estimateTokens(text),
		CreatedAt:     time.Now().UTC(),
		Attempts:      1,
	}, nil
}
