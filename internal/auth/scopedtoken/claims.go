package scopedtoken

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

func (c Claims) HasScope(required string) bool {
	required = strings.TrimSpace(required)
	for _, scope := range c.Scopes {
		if scope == required || scope == "*" {
			return true
		}
	}
	return false
}

func normalizeScopes(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func randomTokenID() (string, error) {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate scoped token ID: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
