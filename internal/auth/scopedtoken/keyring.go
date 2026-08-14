package scopedtoken

/* llmnav/1 module
id=relaydock.auth.scoped-token-keyring
role=Rotate scoped-token signing keys while retaining bounded verification overlap and constant-time secret comparisons.
owns=scoped-token key rotation|verification key overlap|legacy secret deduplication
excludes=token claim encoding|scope authorization policy
search=rotate scoped token key|verify retired signing key|constant time secret compare
invariant=Only the active key signs new tokens while retained keys are verification-only.
invariant=A verification key cannot replace the active key with different secret material.
risk=auth
rel=test>relaydock.auth.scoped-token.contract
stability=contract
*/

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

func New(secret []byte, audience string) (*Service, error) {
	return NewKeyRing(derivedKeyID(secret), secret, nil, audience)
}

// NewKeyRing creates a service that issues with activeSecret and verifies with
// the active key plus every key in verificationKeys. This permits overlap
// during rotation without keeping the retired key as the issuer.
func NewKeyRing(activeKeyID string, activeSecret []byte, verificationKeys map[string][]byte, audience string) (*Service, error) {
	activeKeyID = strings.TrimSpace(activeKeyID)
	audience = strings.TrimSpace(audience)
	if err := validateKey(activeKeyID, activeSecret); err != nil {
		return nil, err
	}
	if audience == "" || len(audience) > maximumIdentityBytes {
		return nil, fmt.Errorf("%w: scoped token audience", core.ErrInvalidConfiguration)
	}
	if len(verificationKeys) > maximumKeyCount {
		return nil, fmt.Errorf("%w: scoped token verification key count exceeds %d", core.ErrInvalidConfiguration, maximumKeyCount)
	}
	keys := make(map[string][]byte, len(verificationKeys)+1)
	keys[activeKeyID] = append([]byte(nil), activeSecret...)
	for keyID, secret := range verificationKeys {
		keyID = strings.TrimSpace(keyID)
		if err := validateKey(keyID, secret); err != nil {
			return nil, err
		}
		if existing, exists := keys[keyID]; exists && !equalSecret(existing, secret) {
			return nil, fmt.Errorf("%w: conflicting scoped token key %q", core.ErrInvalidConfiguration, keyID)
		}
		keys[keyID] = append([]byte(nil), secret...)
	}
	return &Service{
		activeKeyID:     activeKeyID,
		keys:            keys,
		legacySecrets:   uniqueSecrets(keys),
		allowLegacy:     true,
		audience:        audience,
		maximumLifetime: 24 * time.Hour,
		now:             func() time.Time { return time.Now().UTC() },
	}, nil
}

func (s *Service) ActiveKeyID() string {
	if s == nil {
		return ""
	}
	return s.activeKeyID
}

func (s *Service) WithMaximumLifetime(value time.Duration) *Service {
	clone := s.clone()
	if value > 0 {
		clone.maximumLifetime = value
	}
	return clone
}

func (s *Service) WithLegacyVerification(allowed bool) *Service {
	clone := s.clone()
	clone.allowLegacy = allowed
	return clone
}

func (s *Service) WithVerificationKey(keyID string, secret []byte) (*Service, error) {
	if err := validateKey(strings.TrimSpace(keyID), secret); err != nil {
		return nil, err
	}
	clone := s.clone()
	keyID = strings.TrimSpace(keyID)
	if keyID == clone.activeKeyID && !equalSecret(clone.keys[keyID], secret) {
		return nil, fmt.Errorf("%w: verification key cannot replace the active signing secret", core.ErrInvalidConfiguration)
	}
	if clone.keys == nil {
		clone.keys = make(map[string][]byte)
	}
	clone.keys[keyID] = append([]byte(nil), secret...)
	clone.legacySecrets = uniqueSecrets(clone.keys)
	return clone, nil
}

func (s *Service) clone() *Service {
	if s == nil {
		return &Service{}
	}
	clone := *s
	clone.keys = make(map[string][]byte, len(s.keys))
	for keyID, secret := range s.keys {
		clone.keys[keyID] = append([]byte(nil), secret...)
	}
	clone.legacySecrets = make([][]byte, len(s.legacySecrets))
	for index, secret := range s.legacySecrets {
		clone.legacySecrets[index] = append([]byte(nil), secret...)
	}
	return &clone
}

func validateKey(keyID string, secret []byte) error {
	if !validKeyID(keyID) {
		return fmt.Errorf("%w: invalid scoped token key ID %q", core.ErrInvalidConfiguration, keyID)
	}
	if len(secret) < 32 || len(secret) > maximumSecretBytes {
		return fmt.Errorf("%w: scoped token secret must contain 32..%d bytes", core.ErrInvalidConfiguration, maximumSecretBytes)
	}
	return nil
}

func validKeyID(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func derivedKeyID(secret []byte) string {
	digest := sha256.Sum256(secret)
	return "hmac-" + base64.RawURLEncoding.EncodeToString(digest[:9])
}

func uniqueSecrets(keys map[string][]byte) [][]byte {
	ids := make([]string, 0, len(keys))
	for keyID := range keys {
		ids = append(ids, keyID)
	}
	sort.Strings(ids)
	out := make([][]byte, 0, len(ids))
	for _, keyID := range ids {
		secret := keys[keyID]
		duplicate := false
		for _, existing := range out {
			if equalSecret(existing, secret) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, append([]byte(nil), secret...))
		}
	}
	return out
}

func equalSecret(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var different byte
	for index := range left {
		different |= left[index] ^ right[index]
	}
	return different == 0
}
