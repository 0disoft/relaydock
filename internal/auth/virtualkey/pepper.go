package virtualkey

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
)

const maximumPepperBytes = 4096

// SecretResolver is the read-only secret-reference capability needed to load
// virtual-key pepper material in managed server deployments.
type SecretResolver interface {
	Resolve(context.Context, string) ([]byte, error)
}

// PepperConfigured reports whether any supported pepper source is present.
func PepperConfigured(raw, encoded, reference string) bool {
	return strings.TrimSpace(raw) != "" || strings.TrimSpace(encoded) != "" || strings.TrimSpace(reference) != ""
}

// ResolvePepper preserves the existing encoded/raw environment precedence and
// consults a server-secret reference only when neither direct value is set.
func ResolvePepper(ctx context.Context, resolver SecretResolver, raw, encoded, reference string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw = strings.TrimSpace(raw)
	encoded = strings.TrimSpace(encoded)
	var pepper []byte
	var err error
	switch {
	case encoded != "":
		pepper, err = decodePepperBase64(encoded)
		if err != nil {
			return nil, err
		}
	case raw != "":
		pepper = []byte(raw)
	case strings.TrimSpace(reference) != "":
		if resolver == nil {
			return nil, fmt.Errorf("%w: GATEWAY_VIRTUAL_KEY_PEPPER_REF requires a server-secret resolver", core.ErrInvalidConfiguration)
		}
		pepper, err = resolver.Resolve(ctx, reference)
		if err != nil {
			return nil, fmt.Errorf("resolve virtual-key pepper reference: %w", err)
		}
	default:
		return nil, fmt.Errorf("%w: GATEWAY_VIRTUAL_KEY_PEPPER, GATEWAY_VIRTUAL_KEY_PEPPER_B64, or GATEWAY_VIRTUAL_KEY_PEPPER_REF is required", core.ErrInvalidConfiguration)
	}
	if len(pepper) < 32 || len(pepper) > maximumPepperBytes {
		clear(pepper)
		return nil, fmt.Errorf("%w: virtual-key pepper must contain between 32 and %d bytes", core.ErrInvalidConfiguration, maximumPepperBytes)
	}
	return pepper, nil
}

func decodePepperBase64(encoded string) ([]byte, error) {
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: decode GATEWAY_VIRTUAL_KEY_PEPPER_B64", core.ErrInvalidConfiguration)
	}
	return decoded, nil
}
