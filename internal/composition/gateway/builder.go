package gateway

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/0disoft/relaydock/internal/credentials"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/compiler"
	"github.com/0disoft/relaydock/internal/protocol/defaults"
	"github.com/0disoft/relaydock/internal/provider"
	anthropicprovider "github.com/0disoft/relaydock/internal/provider/anthropic"
	"github.com/0disoft/relaydock/internal/provider/deepseek"
	googleprovider "github.com/0disoft/relaydock/internal/provider/google"
	"github.com/0disoft/relaydock/internal/provider/mock"
	"github.com/0disoft/relaydock/internal/provider/openai"
	"github.com/0disoft/relaydock/internal/provider/openaicompatible"
	"github.com/0disoft/relaydock/internal/provider/openrouter"
	"github.com/0disoft/relaydock/internal/routing"
	runtimegateway "github.com/0disoft/relaydock/internal/runtime"
	"github.com/0disoft/relaydock/internal/transport/httpgateway"
)

type Runtime struct {
	Gateway    *runtimegateway.Gateway
	Processor  httpgateway.RuntimeProcessor
	Candidates *CandidateSource
	Models     []string
}

func BuildFromEnvironment() (*Runtime, error) {
	return BuildFromEnvironmentWithCredentials(context.Background(), nil)
}

func BuildFromEnvironmentWithCredentials(ctx context.Context, credentialStore credentials.Store) (*Runtime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	registry := provider.NewRegistry()
	definitions := make(map[string]providerDefinition)
	register := func(adapter provider.Adapter, protocol canonical.Protocol, configured bool) {
		registry.Register(adapter)
		definitions[adapter.Name()] = providerDefinition{Adapter: adapter, Protocol: protocol, Configured: configured}
	}

	openAIKey, openAIConfigured, err := resolveProviderCredential(ctx, credentialStore, "openai")
	if err != nil {
		return nil, err
	}
	anthropicKey, anthropicConfigured, err := resolveProviderCredential(ctx, credentialStore, "anthropic")
	if err != nil {
		return nil, err
	}
	googleKey, googleConfigured, err := resolveProviderCredential(ctx, credentialStore, "google")
	if err != nil {
		return nil, err
	}
	deepSeekKey, deepSeekConfigured, err := resolveProviderCredential(ctx, credentialStore, "deepseek")
	if err != nil {
		return nil, err
	}
	openRouterKey, openRouterConfigured, err := resolveProviderCredential(ctx, credentialStore, "openrouter")
	if err != nil {
		return nil, err
	}
	compatibleKey, _, err := resolveProviderCredential(ctx, credentialStore, "openai-compatible")
	if err != nil {
		return nil, err
	}
	compatibleConfigured := strings.TrimSpace(os.Getenv("OPENAI_COMPATIBLE_BASE_URL")) != ""
	realProviderConfigured := openAIConfigured || anthropicConfigured || googleConfigured || deepSeekConfigured || openRouterConfigured || compatibleConfigured
	localEchoEnabled, err := optionalBooleanEnvironment("GATEWAY_ENABLE_LOCAL_ECHO", !realProviderConfigured)
	if err != nil {
		return nil, err
	}

	mockAdapter := mock.New("Local mock provider is active.")
	register(mockAdapter, canonical.ProtocolOpenAIResponses, localEchoEnabled)

	openAIAdapter := openai.NewWithConfig(os.Getenv("OPENAI_BASE_URL"), openAIKey)
	register(openAIAdapter, canonical.ProtocolOpenAIResponses, openAIConfigured)
	anthropicAdapter := anthropicprovider.NewWithConfig(os.Getenv("ANTHROPIC_BASE_URL"), anthropicKey)
	register(anthropicAdapter, canonical.ProtocolAnthropicMessages, anthropicConfigured)
	googleAdapter := googleprovider.NewWithConfig(os.Getenv("GOOGLE_GENERATIVE_LANGUAGE_BASE_URL"), googleKey)
	register(googleAdapter, canonical.ProtocolGeminiGenerate, googleConfigured)
	deepSeekAdapter := deepseek.NewWithConfig(os.Getenv("DEEPSEEK_BASE_URL"), deepSeekKey)
	register(deepSeekAdapter, canonical.ProtocolOpenAIChat, deepSeekConfigured)
	openRouterAdapter := openrouter.NewWithConfig(os.Getenv("OPENROUTER_BASE_URL"), openRouterKey)
	register(openRouterAdapter, canonical.ProtocolOpenAIChat, openRouterConfigured)
	compatibleAdapter := openaicompatible.NewWithConfig(os.Getenv("OPENAI_COMPATIBLE_BASE_URL"), compatibleKey)
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
