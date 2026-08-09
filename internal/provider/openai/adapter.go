package openai

import (
	"context"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/stream"
	"github.com/0disoft/relaydock/internal/provider"
	"github.com/0disoft/relaydock/internal/provider/httpadapter"
	"os"
)

type Adapter struct{ inner *httpadapter.Adapter }

func New() *Adapter { return NewWithConfig(os.Getenv("OPENAI_BASE_URL"), os.Getenv("OPENAI_API_KEY")) }
func NewWithConfig(baseURL, apiKey string) *Adapter {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &Adapter{inner: httpadapter.New(httpadapter.Config{Name: "openai", BaseURL: baseURL, APIKey: apiKey, Paths: map[canonical.Protocol]string{canonical.ProtocolOpenAIResponses: "/v1/responses", canonical.ProtocolOpenAIChat: "/v1/chat/completions"}, Capabilities: canonical.CapabilitySet{canonical.CapabilityTextInput: true, canonical.CapabilityImageInput: true, canonical.CapabilityToolCalling: true, canonical.CapabilityParallelTools: true, canonical.CapabilityStructuredOutput: true, canonical.CapabilityReasoning: true, canonical.CapabilityContinuation: true, canonical.CapabilityPromptCaching: true}})}
}
func (a *Adapter) Name() string { return a.inner.Name() }
func (a *Adapter) Capabilities(model string) canonical.CapabilitySet {
	return a.inner.Capabilities(model)
}
func (a *Adapter) Execute(ctx context.Context, r provider.AttemptRequest) (provider.AttemptStream, error) {
	return a.inner.Execute(ctx, r)
}
func (a *Adapter) ParseUsage(ctx context.Context, r provider.AttemptResult) (stream.Usage, error) {
	return a.inner.ParseUsage(ctx, r)
}
