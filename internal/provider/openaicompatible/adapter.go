package openaicompatible

import (
	"context"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/stream"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
	"github.com/your-org/ai-runtime-gateway/internal/provider/httpadapter"
	"os"
)

type Adapter struct{ inner *httpadapter.Adapter }

func New() *Adapter {
	return NewWithConfig(os.Getenv("OPENAI_COMPATIBLE_BASE_URL"), os.Getenv("OPENAI_COMPATIBLE_API_KEY"))
}
func NewWithConfig(baseURL, apiKey string) *Adapter {
	caps := canonical.CapabilitySet{canonical.CapabilityTextInput: true, canonical.CapabilityImageInput: true, canonical.CapabilityToolCalling: true, canonical.CapabilityParallelTools: true, canonical.CapabilityStructuredOutput: true, canonical.CapabilityReasoning: true}
	return &Adapter{inner: httpadapter.New(httpadapter.Config{Name: "openai-compatible", BaseURL: baseURL, APIKey: apiKey, AllowAnonymous: apiKey == "" && baseURL != "", Paths: map[canonical.Protocol]string{canonical.ProtocolOpenAIChat: "/v1/chat/completions", canonical.ProtocolOpenAIResponses: "/v1/responses"}, Capabilities: caps})}
}
func (a *Adapter) Name() string                                  { return a.inner.Name() }
func (a *Adapter) Capabilities(m string) canonical.CapabilitySet { return a.inner.Capabilities(m) }
func (a *Adapter) Execute(c context.Context, r provider.AttemptRequest) (provider.AttemptStream, error) {
	return a.inner.Execute(c, r)
}
func (a *Adapter) ParseUsage(c context.Context, r provider.AttemptResult) (stream.Usage, error) {
	return a.inner.ParseUsage(c, r)
}
