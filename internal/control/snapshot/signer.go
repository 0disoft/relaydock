package snapshot

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
)

type Verifier interface {
	Verify(context.Context, Snapshot) error
}

type Signer interface {
	Verifier
	Sign(context.Context, Snapshot) (Snapshot, error)
}

type KeyIDProvider interface {
	SigningKeyID() string
}

type Ed25519Signer struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	KeyID      string
}

func NewEd25519Signer(private ed25519.PrivateKey, public ed25519.PublicKey) Ed25519Signer {
	return NewEd25519SignerWithKeyID("", private, public)
}

func NewEd25519SignerWithKeyID(keyID string, private ed25519.PrivateKey, public ed25519.PublicKey) Ed25519Signer {
	if public == nil && len(private) == ed25519.PrivateKeySize {
		public = private.Public().(ed25519.PublicKey)
	}
	keyID = strings.TrimSpace(keyID)
	if keyID == "" && len(public) == ed25519.PublicKeySize {
		keyID = DeriveKeyID(public)
	}
	return Ed25519Signer{PrivateKey: private, PublicKey: public, KeyID: keyID}
}

func (s Ed25519Signer) SigningKeyID() string { return strings.TrimSpace(s.KeyID) }

func (s Ed25519Signer) Sign(ctx context.Context, value Snapshot) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if len(s.PrivateKey) != ed25519.PrivateKeySize || len(s.PublicKey) != ed25519.PublicKeySize {
		return Snapshot{}, fmt.Errorf("%w: Ed25519 signing key", core.ErrInvalidConfiguration)
	}
	keyID := s.SigningKeyID()
	if !validSigningKeyID(keyID) {
		return Snapshot{}, fmt.Errorf("%w: Ed25519 signing key ID", core.ErrInvalidConfiguration)
	}
	value.SigningKeyID = keyID
	payload, err := canonicalBytes(value)
	if err != nil {
		return Snapshot{}, err
	}
	value.Signature = ed25519.Sign(s.PrivateKey, payload)
	return value, nil
}

func (s Ed25519Signer) Verify(ctx context.Context, value Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(s.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: Ed25519 public key", core.ErrInvalidConfiguration)
	}
	if configuredKeyID := s.SigningKeyID(); !validSigningKeyID(configuredKeyID) {
		return fmt.Errorf("%w: Ed25519 signing key ID", core.ErrInvalidConfiguration)
	}
	if keyID := strings.TrimSpace(value.SigningKeyID); keyID != "" && (!validSigningKeyID(keyID) || keyID != s.SigningKeyID()) {
		return core.ErrUnauthorized
	}
	return verifyEd25519(value, s.PublicKey)
}

func DeriveKeyID(public ed25519.PublicKey) string {
	if len(public) != ed25519.PublicKeySize {
		return ""
	}
	sum := sha256.Sum256(public)
	return "ed25519-" + base64.RawURLEncoding.EncodeToString(sum[:12])
}

func verifyEd25519(value Snapshot, public ed25519.PublicKey) error {
	if len(value.Signature) != ed25519.SignatureSize {
		return core.ErrUnauthorized
	}
	payload, err := canonicalBytes(value)
	if err != nil {
		return err
	}
	if !ed25519.Verify(public, payload, value.Signature) {
		return core.ErrUnauthorized
	}
	return nil
}

func canonicalBytes(value Snapshot) ([]byte, error) {
	value.Signature = nil
	return json.Marshal(value)
}
