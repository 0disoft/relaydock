package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/your-org/ai-runtime-gateway/internal/control/snapshot"
)

func loadControlVerifier() (snapshot.Verifier, []string, error) {
	keys := make([]snapshot.TrustedPublicKey, 0, 4)
	encodedPrimary := firstNonEmpty(
		os.Getenv("GATEWAY_CONTROL_SIGNING_PUBLIC_KEY_B64"),
		os.Getenv("GATEWAY_CONTROL_SIGNING_PUBLIC_KEY"),
	)
	if encodedPrimary != "" {
		public, err := snapshot.ParsePublicKey(encodedPrimary)
		if err != nil {
			return nil, nil, err
		}
		keys = append(keys, snapshot.TrustedPublicKey{
			ID: strings.TrimSpace(os.Getenv("GATEWAY_CONTROL_SIGNING_KEY_ID")), PublicKey: public,
		})
	}
	additional, err := snapshot.ParseTrustedPublicKeys(os.Getenv("GATEWAY_CONTROL_TRUSTED_SIGNING_PUBLIC_KEYS"))
	if err != nil {
		return nil, nil, err
	}
	keys = append(keys, additional...)
	if len(keys) == 0 {
		return nil, nil, fmt.Errorf("GATEWAY_CONTROL_SIGNING_PUBLIC_KEY_B64 or GATEWAY_CONTROL_TRUSTED_SIGNING_PUBLIC_KEYS is required with GATEWAY_CONTROL_URL")
	}
	keys = deduplicateTrustedKeys(keys)
	allowLegacy, err := booleanEnvironment("GATEWAY_CONTROL_ALLOW_LEGACY_SIGNING_KEY_ID", true)
	if err != nil {
		return nil, nil, err
	}
	ring, err := snapshot.NewEd25519KeyRing(keys, allowLegacy)
	if err != nil {
		return nil, nil, err
	}
	return ring, ring.KeyIDs(), nil
}

func deduplicateTrustedKeys(keys []snapshot.TrustedPublicKey) []snapshot.TrustedPublicKey {
	seen := make(map[string]struct{}, len(keys))
	result := make([]snapshot.TrustedPublicKey, 0, len(keys))
	for _, key := range keys {
		id := strings.TrimSpace(key.ID)
		if id == "" {
			id = snapshot.DeriveKeyID(key.PublicKey)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		key.ID = id
		result = append(result, key)
	}
	return result
}
