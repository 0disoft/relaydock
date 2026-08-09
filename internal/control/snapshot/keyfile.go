package snapshot

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/persistence/atomicfile"
)

// LoadOrCreateSigningKey keeps the control-plane trust root stable across
// restarts. The private key is raw Ed25519 private-key bytes and is written
// owner-only. Production deployments may replace this with KMS-backed Signer.
func LoadOrCreateSigningKey(path string) (Ed25519Signer, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Ed25519Signer{}, fmt.Errorf("%w: control signing key path", core.ErrInvalidArgument)
	}
	raw, err := atomicfile.Read(path, ed25519.PrivateKeySize+1)
	if err != nil {
		return Ed25519Signer{}, err
	}
	if len(raw) == ed25519.PrivateKeySize {
		privateKey := append(ed25519.PrivateKey(nil), raw...)
		return NewEd25519Signer(privateKey, nil), nil
	}
	if len(raw) != 0 {
		return Ed25519Signer{}, fmt.Errorf("%w: control signing key has %d bytes", core.ErrInvalidConfiguration, len(raw))
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Ed25519Signer{}, fmt.Errorf("generate control signing key: %w", err)
	}
	if err := atomicfile.Write(path, privateKey, 0o600); err != nil {
		return Ed25519Signer{}, err
	}
	return NewEd25519Signer(privateKey, publicKey), nil
}
