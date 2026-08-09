package snapshot

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
)

func ParsePublicKey(value string) (ed25519.PublicKey, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("%w: control signing public key is empty", core.ErrInvalidConfiguration)
	}
	encodings := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	}
	var raw []byte
	var err error
	for _, encoding := range encodings {
		raw, err = encoding.DecodeString(value)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("decode control signing public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: control signing public key has %d bytes", core.ErrInvalidConfiguration, len(raw))
	}
	return append(ed25519.PublicKey(nil), raw...), nil
}
