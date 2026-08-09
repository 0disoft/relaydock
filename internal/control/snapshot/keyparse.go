package snapshot

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

// ParseSigningPrivateKey accepts an Ed25519 seed or private key encoded with
// base64url/base64, padded or unpadded. It exists so clustered controld
// instances can share one trust root through a secret manager instead of a
// writable local volume.
func ParseSigningPrivateKey(encoded string) (Ed25519Signer, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return Ed25519Signer{}, fmt.Errorf("%w: empty control signing private key", core.ErrInvalidArgument)
	}
	var raw []byte
	var decodeErr error
	for _, encoding := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	} {
		raw, decodeErr = encoding.DecodeString(encoded)
		if decodeErr == nil {
			break
		}
	}
	if decodeErr != nil {
		return Ed25519Signer{}, fmt.Errorf("%w: decode control signing private key", core.ErrInvalidArgument)
	}
	var privateKey ed25519.PrivateKey
	switch len(raw) {
	case ed25519.SeedSize:
		privateKey = ed25519.NewKeyFromSeed(raw)
	case ed25519.PrivateKeySize:
		privateKey = append(ed25519.PrivateKey(nil), raw...)
	default:
		return Ed25519Signer{}, fmt.Errorf("%w: control signing private key has %d decoded bytes", core.ErrInvalidConfiguration, len(raw))
	}
	return NewEd25519Signer(privateKey, nil), nil
}
