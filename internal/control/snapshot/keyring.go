package snapshot

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
)

const maximumTrustedSigningKeys = 64

type TrustedPublicKey struct {
	ID        string            `json:"id"`
	PublicKey ed25519.PublicKey `json:"-"`
}

type encodedTrustedPublicKey struct {
	ID        string `json:"id"`
	PublicKey string `json:"publicKey"`
}

type Ed25519KeyRing struct {
	keys                    map[string]ed25519.PublicKey
	allowLegacyWithoutKeyID bool
}

func NewEd25519KeyRing(keys []TrustedPublicKey, allowLegacyWithoutKeyID bool) (*Ed25519KeyRing, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: empty Ed25519 trust ring", core.ErrInvalidConfiguration)
	}
	if len(keys) > maximumTrustedSigningKeys {
		return nil, fmt.Errorf("%w: trusted Ed25519 key count exceeds %d", core.ErrInvalidConfiguration, maximumTrustedSigningKeys)
	}
	ring := &Ed25519KeyRing{keys: make(map[string]ed25519.PublicKey, len(keys)), allowLegacyWithoutKeyID: allowLegacyWithoutKeyID}
	for _, key := range keys {
		if len(key.PublicKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%w: malformed trusted Ed25519 public key", core.ErrInvalidConfiguration)
		}
		id := strings.TrimSpace(key.ID)
		if id == "" {
			id = DeriveKeyID(key.PublicKey)
		}
		if !validSigningKeyID(id) {
			return nil, fmt.Errorf("%w: invalid trusted signing key ID %q", core.ErrInvalidConfiguration, id)
		}
		if _, exists := ring.keys[id]; exists {
			return nil, fmt.Errorf("%w: duplicate trusted signing key ID %q", core.ErrConflict, id)
		}
		ring.keys[id] = append(ed25519.PublicKey(nil), key.PublicKey...)
	}
	return ring, nil
}

func (r *Ed25519KeyRing) Verify(ctx context.Context, value Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r == nil || len(r.keys) == 0 {
		return fmt.Errorf("%w: empty Ed25519 trust ring", core.ErrInvalidConfiguration)
	}
	keyID := strings.TrimSpace(value.SigningKeyID)
	if keyID != "" {
		public, exists := r.keys[keyID]
		if !exists {
			return core.ErrUnauthorized
		}
		return verifyEd25519(value, public)
	}
	if !r.allowLegacyWithoutKeyID {
		return core.ErrUnauthorized
	}
	for _, public := range r.keys {
		if verifyEd25519(value, public) == nil {
			return nil
		}
	}
	return core.ErrUnauthorized
}

func (r *Ed25519KeyRing) KeyIDs() []string {
	if r == nil {
		return nil
	}
	ids := make([]string, 0, len(r.keys))
	for id := range r.keys {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ParseTrustedPublicKeys accepts either a JSON array
// [{"id":"2026-q3","publicKey":"..."}] or a comma-separated list of
// id=base64url entries. IDs may be omitted in JSON and are then derived from
// the public key fingerprint.
func ParseTrustedPublicKeys(value string) ([]TrustedPublicKey, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	var encoded []encodedTrustedPublicKey
	if strings.HasPrefix(value, "[") {
		decoder := json.NewDecoder(strings.NewReader(value))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&encoded); err != nil {
			return nil, fmt.Errorf("decode trusted signing keys: %w", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			if err == nil {
				return nil, fmt.Errorf("decode trusted signing keys: multiple JSON values")
			}
			return nil, fmt.Errorf("decode trusted signing keys: malformed trailing JSON: %w", err)
		}
	} else {
		for _, entry := range strings.Split(value, ",") {
			id, publicKey, found := strings.Cut(strings.TrimSpace(entry), "=")
			if !found {
				return nil, fmt.Errorf("trusted signing key entry must use id=base64url")
			}
			encoded = append(encoded, encodedTrustedPublicKey{ID: strings.TrimSpace(id), PublicKey: strings.TrimSpace(publicKey)})
		}
	}
	if len(encoded) > maximumTrustedSigningKeys {
		return nil, fmt.Errorf("trusted signing key count exceeds %d", maximumTrustedSigningKeys)
	}
	keys := make([]TrustedPublicKey, 0, len(encoded))
	for _, item := range encoded {
		public, err := ParsePublicKey(item.PublicKey)
		if err != nil {
			return nil, err
		}
		keys = append(keys, TrustedPublicKey{ID: strings.TrimSpace(item.ID), PublicKey: public})
	}
	return keys, nil
}

func validSigningKeyID(value string) bool {
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
