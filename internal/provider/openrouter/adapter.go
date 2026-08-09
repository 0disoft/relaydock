package openrouter

import (
	"context"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/stream"
	"github.com/0disoft/relaydock/internal/provider"
	"github.com/0disoft/relaydock/internal/provider/httpadapter"
	"os"
)

type Adapter struct{ inner *httpadapter.Adapter }

func New() *Adapter {
	return NewWithConfig(os.Getenv("OPENROUTER_BASE_URL"), os.Getenv("OPENROUTER_API_KEY"))
}
func NewWithConfig(baseURL, apiKey string) *Adapter {
	if baseURL == "" {
		baseURL = "https://openrouter.ai"
	}
	headers := map[string]string{}
	if v := os.Getenv("OPENROUTER_HTTP_REFERER"); v != "" {
		headers["HTTP-Referer"] = v
	}
	if v := os.Getenv("OPENROUTER_APP_TITLE"); v != "" {
		headers["X-Title"] = v
	}
	caps := canonical.CapabilitySet{canonical.CapabilityTextInput: true, canonical.CapabilityImageInput: true, canonical.CapabilityToolCalling: true, canonical.CapabilityParallelTools: true, canonical.CapabilityStructuredOutput: true, canonical.CapabilityReasoning: true}
	return &Adapter{inner: httpadapter.New(httpadapter.Config{Name: "openrouter", BaseURL: baseURL, APIKey: apiKey, Headers: headers, Paths: map[canonical.Protocol]string{canonical.ProtocolOpenAIChat: "/api/v1/chat/completions", canonical.ProtocolOpenAIResponses: "/api/v1/responses"}, Capabilities: caps})}
}
func (a *Adapter) Name() string                                  { return a.inner.Name() }
func (a *Adapter) Capabilities(m string) canonical.CapabilitySet { return a.inner.Capabilities(m) }
func (a *Adapter) Execute(c context.Context, r provider.AttemptRequest) (provider.AttemptStream, error) {
	return a.inner.Execute(c, r)
}
func (a *Adapter) ParseUsage(c context.Context, r provider.AttemptResult) (stream.Usage, error) {
	return a.inner.ParseUsage(c, r)
}
