package serversecrets

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/relaydock/internal/core"
)

type testProvider struct {
	scheme   string
	resource string
}

func (p *testProvider) Scheme() string { return p.scheme }

func (p *testProvider) Resolve(ctx context.Context, resource string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.resource = resource
	return []byte("resolved"), nil
}

func TestResolverDispatchesCanonicalReference(t *testing.T) {
	provider := &testProvider{scheme: "test"}
	resolver, err := NewResolver(provider)
	if err != nil {
		t.Fatal(err)
	}
	value, err := resolver.Resolve(context.Background(), "test:resource/name")
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != "resolved" || provider.resource != "resource/name" {
		t.Fatalf("value=%q resource=%q", value, provider.resource)
	}
}

func TestResolverRejectsMalformedAndUnsupportedReferences(t *testing.T) {
	resolver, err := NewResolver(&testProvider{scheme: "test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{"", " test:resource", "TEST:resource", "test:", "test:resource?query", "missing:resource"} {
		_, err := resolver.Resolve(context.Background(), reference)
		if !errors.Is(err, core.ErrInvalidConfiguration) {
			t.Fatalf("reference %q error=%v", reference, err)
		}
	}
}

func TestResolverErrorsDoNotContainResolvedValue(t *testing.T) {
	resolver, err := NewResolver(&testProvider{scheme: "test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolver.Resolve(context.Background(), "missing:super-secret-value")
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
	if strings.Contains(err.Error(), "super-secret-value") {
		t.Fatalf("error exposed resource: %v", err)
	}
}
