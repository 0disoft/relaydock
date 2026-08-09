package gateway

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/compiler"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/defaults"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
	anthropicprovider "github.com/your-org/ai-runtime-gateway/internal/provider/anthropic"
	"github.com/your-org/ai-runtime-gateway/internal/provider/deepseek"
	googleprovider "github.com/your-org/ai-runtime-gateway/internal/provider/google"
	"github.com/your-org/ai-runtime-gateway/internal/provider/mock"
	"github.com/your-org/ai-runtime-gateway/internal/provider/openai"
	"github.com/your-org/ai-runtime-gateway/internal/provider/openaicompatible"
	"github.com/your-org/ai-runtime-gateway/internal/provider/openrouter"
	"github.com/your-org/ai-runtime-gateway/internal/routing"
	runtimegateway "github.com/your-org/ai-runtime-gateway/internal/runtime"
	"github.com/your-org/ai-runtime-gateway/internal/transport/httpgateway"
)

type Runtime struct {
	Gateway    *runtimegateway.Gateway
	Processor  httpgateway.RuntimeProcessor
	Candidates *CandidateSource
	Models     []string
}

func BuildFromEnvironment() (*Runtime, error) {
	registry := provider.NewRegistry()
	definitions := make(map[string]providerDefinition)
	register := func(adapter provider.Adapter, protocol canonical.Protocol, configured bool) {
		registry.Register(adapter)
		definitions[adapter.Name()] = providerDefinition{Adapter: adapter, Protocol: protocol, Configured: configured}
	}

	openAIConfigured := configured("OPENAI_API_KEY")
	anthropicConfigured := configured("ANTHROPIC_API_KEY")
	googleConfigured := configuredAny("GOOGLE_API_KEY", "GEMINI_API_KEY")
	deepSeekConfigured := configured("DEEPSEEK_API_KEY")
	openRouterConfigured := configured("OPENROUTER_API_KEY")
	compatibleConfigured := strings.TrimSpace(os.Getenv("OPENAI_COMPATIBLE_BASE_URL")) != ""
	realProviderConfigured := openAIConfigured || anthropicConfigured || googleConfigured || deepSeekConfigured || openRouterConfigured || compatibleConfigured
	localEchoEnabled, err := optionalBooleanEnvironment("GATEWAY_ENABLE_LOCAL_ECHO", !realProviderConfigured)
	if err != nil {
		return nil, err
	}

	mockAdapter := mock.New("Local mock provider is active.")
	register(mockAdapter, canonical.ProtocolOpenAIResponses, localEchoEnabled)

	openAIAdapter := openai.New()
	register(openAIAdapter, canonical.ProtocolOpenAIResponses, openAIConfigured)
	anthropicAdapter := anthropicprovider.New()
	register(anthropicAdapter, canonical.ProtocolAnthropicMessages, anthropicConfigured)
	googleAdapter := googleprovider.New()
	register(googleAdapter, canonical.ProtocolGeminiGenerate, googleConfigured)
	deepSeekAdapter := deepseek.New()
	register(deepSeekAdapter, canonical.ProtocolOpenAIChat, deepSeekConfigured)
	openRouterAdapter := openrouter.New()
	register(openRouterAdapter, canonical.ProtocolOpenAIChat, openRouterConfigured)
	compatibleAdapter := openaicompatible.New()
	register(compatibleAdapter, canonical.ProtocolOpenAIChat, compatibleConfigured)

	routes, err := loadRouteConfig(os.Getenv("GATEWAY_ROUTES_FILE"), os.Getenv("GATEWAY_ROUTES_JSON"))
	if err != nil {
		return nil, err
	}
	candidates, err := newCandidateSource(definitions, routes, os.Getenv("GATEWAY_DEFAULT_PROVIDER"))
	if err != nil {
		return nil, err
	}

	gateway := runtimegateway.NewGateway()
	gateway.Compiler = defaults.Compiler()
	gateway.Router = routing.New(nil)
	gateway.Providers = registry
	gateway.Candidates = candidates
	gateway.Leases = routing.NewMemoryLeaseManager()
	gateway.LossMode = compiler.LossModeStrict
	gateway.MaximumAttempts = 3

	return &Runtime{
		Gateway: gateway,
		Processor: httpgateway.RuntimeProcessor{
			Gateway:             gateway,
			PriceRevisionSource: candidates.PriceRevisionID,
		},
		Candidates: candidates,
		Models:     candidates.Models(),
	}, nil
}

func configured(name string) bool {
	return strings.TrimSpace(os.Getenv(name)) != ""
}

func configuredAny(names ...string) bool {
	for _, name := range names {
		if configured(name) {
			return true
		}
	}
	return false
}

func optionalBooleanEnvironment(name string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", name, err)
	}
	return value, nil
}
