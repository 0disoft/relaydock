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
	id          string
	environment []string
}

var providerCredentialDefinitions = []providerCredentialDefinition{
	{id: "openai", environment: []string{"OPENAI_API_KEY"}},
	{id: "anthropic", environment: []string{"ANTHROPIC_API_KEY"}},
	{id: "google", environment: []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"}},
	{id: "deepseek", environment: []string{"DEEPSEEK_API_KEY"}},
	{id: "openrouter", environment: []string{"OPENROUTER_API_KEY"}},
	{id: "openai-compatible", environment: []string{"OPENAI_COMPATIBLE_API_KEY"}},
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

func resolveProviderCredential(ctx context.Context, store credentials.Store, providerID string) (string, bool, error) {
	definition, ok := providerCredentialDefinitionFor(providerID)
	if !ok {
		return "", false, fmt.Errorf("%w: unsupported credential provider", core.ErrInvalidArgument)
	}
	if value, configured := firstEnvironmentValue(definition.environment); configured {
		return value, true, nil
	}
	if store == nil {
		return "", false, nil
	}
	ref, _ := ProviderCredentialReference(providerID)
	raw, err := store.Get(ctx, ref)
	if errors.Is(err, core.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("resolve %s provider credential: %w", definition.id, err)
	}
	defer zeroProviderCredential(raw)
	value := string(raw)
	if value == "" || strings.TrimSpace(value) != value {
		return "", false, fmt.Errorf("%w: %s provider credential contains surrounding whitespace", core.ErrInvalidConfiguration, definition.id)
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
