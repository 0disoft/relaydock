package google

import (
	"context"
	"fmt"
	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/stream"
	"github.com/0disoft/relaydock/internal/provider"
	"github.com/0disoft/relaydock/internal/provider/httpadapter"
	"net/url"
	"os"
	"strings"
)

type Adapter struct{ inner *httpadapter.Adapter }

func New() *Adapter {
	return NewWithConfig(os.Getenv("GOOGLE_GENERATIVE_LANGUAGE_BASE_URL"), firstNonEmpty(os.Getenv("GOOGLE_API_KEY"), os.Getenv("GEMINI_API_KEY")))
}
func NewWithConfig(baseURL, apiKey string) *Adapter {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	caps := canonical.CapabilitySet{canonical.CapabilityTextInput: true, canonical.CapabilityImageInput: true, canonical.CapabilityToolCalling: true, canonical.CapabilityParallelTools: true, canonical.CapabilityStructuredOutput: true, canonical.CapabilityReasoning: true, canonical.CapabilityPromptCaching: true}
	builder := func(r provider.AttemptRequest) (string, error) {
		if r.Request.Protocol != canonical.ProtocolGeminiGenerate {
			return "", core.ErrCapabilityMismatch
		}
		model := strings.TrimSpace(r.Model)
		if model == "" {
			return "", fmt.Errorf("%w: Gemini model", core.ErrInvalidArgument)
		}
		return "/v1beta/models/" + url.PathEscape(model) + ":streamGenerateContent?alt=sse", nil
	}
	return &Adapter{inner: httpadapter.New(httpadapter.Config{Name: "google", BaseURL: baseURL, APIKey: apiKey, AuthHeader: "x-goog-api-key", AuthPrefix: "", PathBuilder: builder, Capabilities: caps})}
}
func (a *Adapter) Name() string                                  { return a.inner.Name() }
func (a *Adapter) Capabilities(m string) canonical.CapabilitySet { return a.inner.Capabilities(m) }
func (a *Adapter) Execute(c context.Context, r provider.AttemptRequest) (provider.AttemptStream, error) {
	return a.inner.Execute(c, r)
}
func (a *Adapter) ParseUsage(c context.Context, r provider.AttemptResult) (stream.Usage, error) {
	return a.inner.ParseUsage(c, r)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
