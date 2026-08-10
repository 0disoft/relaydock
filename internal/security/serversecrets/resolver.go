package serversecrets

import (
	"context"
	"fmt"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
)

// Provider resolves one server-secret reference scheme. Implementations must
// return newly owned bytes and must not include secret values in errors.
type Provider interface {
	Scheme() string
	Resolve(context.Context, string) ([]byte, error)
}

// Resolver dispatches canonical server-secret references to registered
// providers. It is read-only because workload identity should grant services
// access to existing secrets, not permission to mutate them at startup.
type Resolver struct {
	providers map[string]Provider
}

// NewDefaultResolver registers the production server-secret providers
// supported by this build.
func NewDefaultResolver() (*Resolver, error) {
	return NewResolver(NewGCPSecretManager())
}

func NewResolver(providers ...Provider) (*Resolver, error) {
	resolver := &Resolver{providers: make(map[string]Provider, len(providers))}
	for _, provider := range providers {
		if provider == nil {
			return nil, fmt.Errorf("%w: nil server-secret provider", core.ErrInvalidConfiguration)
		}
		scheme := strings.ToLower(strings.TrimSpace(provider.Scheme()))
		if !validScheme(scheme) {
			return nil, fmt.Errorf("%w: invalid server-secret provider scheme", core.ErrInvalidConfiguration)
		}
		if _, exists := resolver.providers[scheme]; exists {
			return nil, fmt.Errorf("%w: duplicate server-secret provider scheme %q", core.ErrInvalidConfiguration, scheme)
		}
		resolver.providers[scheme] = provider
	}
	return resolver, nil
}

func (r *Resolver) Resolve(ctx context.Context, reference string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	scheme, resource, err := parseReference(reference)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: server-secret resolver is unavailable", core.ErrInvalidConfiguration)
	}
	provider, ok := r.providers[scheme]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported server-secret scheme %q", core.ErrInvalidConfiguration, scheme)
	}
	value, err := provider.Resolve(ctx, resource)
	if err != nil {
		return nil, fmt.Errorf("resolve %s server secret: %w", scheme, err)
	}
	return value, nil
}

func parseReference(reference string) (string, string, error) {
	if reference == "" || strings.TrimSpace(reference) != reference {
		return "", "", fmt.Errorf("%w: server-secret reference must not be empty or contain surrounding whitespace", core.ErrInvalidConfiguration)
	}
	scheme, resource, found := strings.Cut(reference, ":")
	if !found || !validScheme(scheme) || scheme != strings.ToLower(scheme) {
		return "", "", fmt.Errorf("%w: server-secret reference requires a lowercase URI scheme", core.ErrInvalidConfiguration)
	}
	if resource == "" || strings.ContainsAny(resource, "?#\r\n\t") {
		return "", "", fmt.Errorf("%w: malformed server-secret resource", core.ErrInvalidConfiguration)
	}
	return scheme, resource, nil
}

func validScheme(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, char := range value[1:] {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '+' && char != '-' && char != '.' {
			return false
		}
	}
	return true
}
