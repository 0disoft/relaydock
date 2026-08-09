package provider

import (
	"context"
	"io"

	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/compiler"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/stream"
)

type Adapter interface {
	Name() string
	Capabilities(model string) canonical.CapabilitySet
	Execute(context.Context, AttemptRequest) (AttemptStream, error)
	ParseUsage(context.Context, AttemptResult) (stream.Usage, error)
}

type AttemptRequest struct {
	ProviderAccountID string
	Model             string
	Request           compiler.EncodedRequest
}

type AttemptStream interface {
	Next(context.Context) (stream.Event, error)
	Close() error
}

type AttemptResult struct {
	Headers map[string][]string
	Body    io.Reader
}
