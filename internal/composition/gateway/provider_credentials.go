package gateway

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/credentials"
)

type providerCredentialDefinition struct {
	id                   string
	environment          []string
	referenceEnvironment string
}

var providerCredentialDefinitions = []providerCredentialDefinition{
	{id: "openai", environment: []string{"OPENAI_API_KEY"}, referenceEnvironment: "GATEWAY_OPENAI_API_KEY_REF"},
	{id: "anthropic", environment: []string{"ANTHROPIC_API_KEY"}, referenceEnvironment: "GATEWAY_ANTHROPIC_API_KEY_REF"},
	{id: "google", environment: []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"}, referenceEnvironment: "GATEWAY_GOOGLE_API_KEY_REF"},
	{id: "deepseek", environment: []string{"DEEPSEEK_API_KEY"}, referenceEnvironment: "GATEWAY_DEEPSEEK_API_KEY_REF"},
	{id: "openrouter", environment: []string{"OPENROUTER_API_KEY"}, referenceEnvironment: "GATEWAY_OPENROUTER_API_KEY_REF"},
	{id: "openai-compatible", environment: []string{"OPENAI_COMPATIBLE_API_KEY"}, referenceEnvironment: "GATEWAY_OPENAI_COMPATIBLE_API_KEY_REF"},
}

// ServerSecretResolver resolves read-only secret references for server
// deployments. Desktop composition intentionally leaves this source unset.
type ServerSecretResolver interface {
	Resolve(context.Context, string) ([]byte, error)
}

func ProviderCredentialReference(providerID string) (credentials.Reference, error) {
	definition, ok := providerCredentialDefinitionFor(providerID)
	if !ok {
		return credentials.Reference{}, fmt.Errorf("%w: unsupported credential provider", core.ErrInvalidArgument)
	}
	return credentials.Reference{ID: "api-key", Provider: definition.id, Account: "default"}, nil
}

func ProviderCredentialEnvironmentConfigured(providerID string) bool {
	definition, ok := providerCredentialDefinitionFor(providerID)
	if !ok {
		return false
	}
	_, configured := firstEnvironmentValue(definition.environment)
	return configured
}

func resolveProviderCredential(ctx context.Context, store credentials.Store, resolver ServerSecretResolver, providerID string) (string, bool, error) {
	definition, ok := providerCredentialDefinitionFor(providerID)
	if !ok {
		return "", false, fmt.Errorf("%w: unsupported credential provider", core.ErrInvalidArgument)
	}
	if value, configured := firstEnvironmentValue(definition.environment); configured {
		return value, true, nil
	}
	if store != nil {
		ref, _ := ProviderCredentialReference(providerID)
		raw, err := store.Get(ctx, ref)
		if err == nil {
			return validatedProviderCredential(definition.id, raw)
		}
		if !errors.Is(err, core.ErrNotFound) {
			return "", false, fmt.Errorf("resolve %s provider credential: %w", definition.id, err)
		}
	}
	reference := os.Getenv(definition.referenceEnvironment)
	if strings.TrimSpace(reference) == "" {
		return "", false, nil
	}
	if resolver == nil {
		return "", false, fmt.Errorf("%w: %s requires a server-secret resolver", core.ErrInvalidConfiguration, definition.referenceEnvironment)
	}
	raw, err := resolver.Resolve(ctx, reference)
	if err != nil {
		return "", false, fmt.Errorf("resolve %s provider credential reference: %w", definition.id, err)
	}
	return validatedProviderCredential(definition.id, raw)
}

func validatedProviderCredential(providerID string, raw []byte) (string, bool, error) {
	defer zeroProviderCredential(raw)
	if len(raw) == 0 || len(raw) > 2048 {
		return "", false, fmt.Errorf("%w: %s provider credential size is outside the supported range", core.ErrInvalidConfiguration, providerID)
	}
	value := string(raw)
	if strings.TrimSpace(value) != value {
		return "", false, fmt.Errorf("%w: %s provider credential contains surrounding whitespace", core.ErrInvalidConfiguration, providerID)
	}
	return value, true, nil
}

func providerCredentialDefinitionFor(providerID string) (providerCredentialDefinition, bool) {
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	for _, definition := range providerCredentialDefinitions {
		if definition.id == providerID {
			return definition, true
		}
	}
	return providerCredentialDefinition{}, false
}

func firstEnvironmentValue(names []string) (string, bool) {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value, true
		}
	}
	return "", false
}

func zeroProviderCredential(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
