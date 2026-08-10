package virtualkey

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/relaydock/internal/core"
)

type pepperResolver struct {
	value     []byte
	reference string
	calls     int
}

func (r *pepperResolver) Resolve(ctx context.Context, reference string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.calls++
	r.reference = reference
	return append([]byte(nil), r.value...), nil
}

func TestResolvePepperUsesServerSecretReference(t *testing.T) {
	resolver := &pepperResolver{value: []byte(strings.Repeat("p", 32))}
	pepper, err := ResolvePepper(context.Background(), resolver, "", "", "gcp-sm:projects/p/secrets/pepper/versions/latest")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(pepper)
	if resolver.calls != 1 || resolver.reference != "gcp-sm:projects/p/secrets/pepper/versions/latest" {
		t.Fatalf("calls=%d reference=%q", resolver.calls, resolver.reference)
	}
	if string(pepper) != strings.Repeat("p", 32) {
		t.Fatalf("unexpected resolved pepper value with length %d", len(pepper))
	}
}

func TestResolvePepperKeepsDirectEnvironmentPrecedence(t *testing.T) {
	resolver := &pepperResolver{value: []byte(strings.Repeat("r", 32))}
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
	pepper, err := ResolvePepper(
		context.Background(), resolver,
		strings.Repeat("r", 32), encoded,
		"gcp-sm:projects/p/secrets/pepper/versions/latest",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(pepper)
	if resolver.calls != 0 {
		t.Fatal("server-secret resolver ran despite a direct environment value")
	}
	if string(pepper) != strings.Repeat("x", 32) {
		t.Fatalf("encoded environment did not keep precedence: %q", pepper)
	}
}

func TestResolvePepperFailsClosedForInvalidSources(t *testing.T) {
	secretText := "do-not-copy-this-pepper"
	resolver := &pepperResolver{value: []byte(secretText)}
	oversizedResolver := &pepperResolver{value: []byte(strings.Repeat("p", maximumPepperBytes+1))}
	for _, test := range []struct {
		name      string
		resolver  SecretResolver
		raw       string
		encoded   string
		reference string
	}{
		{name: "missing"},
		{name: "malformed base64", encoded: "%%%"},
		{name: "short reference", resolver: resolver, reference: "gcp-sm:projects/p/secrets/pepper/versions/latest"},
		{name: "oversized reference", resolver: oversizedResolver, reference: "gcp-sm:projects/p/secrets/pepper/versions/latest"},
		{name: "missing resolver", reference: "gcp-sm:projects/p/secrets/pepper/versions/latest"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ResolvePepper(context.Background(), test.resolver, test.raw, test.encoded, test.reference)
			if !errors.Is(err, core.ErrInvalidConfiguration) {
				t.Fatalf("error=%v", err)
			}
			if strings.Contains(err.Error(), secretText) {
				t.Fatalf("error exposed pepper material: %v", err)
			}
		})
	}
}

func TestResolvePepperHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := &pepperResolver{value: []byte(strings.Repeat("p", 32))}
	_, err := ResolvePepper(ctx, resolver, "", "", "gcp-sm:projects/p/secrets/pepper/versions/latest")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	if resolver.calls != 0 {
		t.Fatalf("resolver calls=%d", resolver.calls)
	}
}
