package anthropic

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
	return NewWithConfig(os.Getenv("ANTHROPIC_BASE_URL"), os.Getenv("ANTHROPIC_API_KEY"))
}
func NewWithConfig(baseURL, apiKey string) *Adapter {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	caps := canonical.CapabilitySet{canonical.CapabilityTextInput: true, canonical.CapabilityImageInput: true, canonical.CapabilityToolCalling: true, canonical.CapabilityParallelTools: true, canonical.CapabilityReasoning: true, canonical.CapabilityPromptCaching: true}
	return &Adapter{inner: httpadapter.New(httpadapter.Config{Name: "anthropic", BaseURL: baseURL, APIKey: apiKey, AuthHeader: "x-api-key", AuthPrefix: "", Headers: map[string]string{"anthropic-version": "2023-06-01"}, Paths: map[canonical.Protocol]string{canonical.ProtocolAnthropicMessages: "/v1/messages"}, Capabilities: caps})}
}
func (a *Adapter) Name() string                                  { return a.inner.Name() }
func (a *Adapter) Capabilities(m string) canonical.CapabilitySet { return a.inner.Capabilities(m) }
func (a *Adapter) Execute(c context.Context, r provider.AttemptRequest) (provider.AttemptStream, error) {
	return a.inner.Execute(c, r)
}
func (a *Adapter) ParseUsage(c context.Context, r provider.AttemptResult) (stream.Usage, error) {
	return a.inner.ParseUsage(c, r)
}
