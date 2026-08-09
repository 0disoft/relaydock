package scopedtoken

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

func KeyID(secret []byte) string {
	return derivedKeyID(secret)
}

func DecodeBase64Secret(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("base64 secret is empty")
	}
	for _, encoding := range []*base64.Encoding{
		base64.RawStdEncoding,
		base64.StdEncoding,
		base64.RawURLEncoding,
		base64.URLEncoding,
	} {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			if len(decoded) > maximumSecretBytes {
				return nil, fmt.Errorf("decoded secret exceeds %d bytes", maximumSecretBytes)
			}
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("decode base64 secret")
}

// ParseBase64KeyMap decodes a JSON object whose values are base64-encoded
// secrets. Validation of key IDs and secret lengths is performed by NewKeyRing.
func ParseBase64KeyMap(value string) (map[string][]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	var encoded map[string]string
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encoded); err != nil {
		return nil, fmt.Errorf("decode scoped token verification key map: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode scoped token verification key map: trailing JSON")
	}
	if len(encoded) > maximumKeyCount {
		return nil, fmt.Errorf("scoped token verification key count exceeds %d", maximumKeyCount)
	}
	keys := make(map[string][]byte, len(encoded))
	for keyID, raw := range encoded {
		normalizedID := strings.TrimSpace(keyID)
		if _, exists := keys[normalizedID]; exists {
			return nil, fmt.Errorf("duplicate normalized scoped token verification key %q", normalizedID)
		}
		secret, err := DecodeBase64Secret(raw)
		if err != nil {
			return nil, fmt.Errorf("decode scoped token verification key %q: %w", keyID, err)
		}
		keys[normalizedID] = secret
	}
	return keys, nil
}
