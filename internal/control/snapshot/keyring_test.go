package snapshot

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

func TestEd25519KeyRingAcceptsOverlappingRotationKeys(t *testing.T) {
	t.Parallel()
	oldPublic, oldPrivate, _ := ed25519.GenerateKey(rand.Reader)
	newPublic, newPrivate, _ := ed25519.GenerateKey(rand.Reader)
	oldSigner := NewEd25519SignerWithKeyID("old", oldPrivate, oldPublic)
	newSigner := NewEd25519SignerWithKeyID("new", newPrivate, newPublic)
	ring, err := NewEd25519KeyRing([]TrustedPublicKey{
		{ID: "old", PublicKey: oldPublic}, {ID: "new", PublicKey: newPublic},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	value := Snapshot{Revision: 1, GeneratedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(time.Hour).UTC(), Models: []ModelRoute{{VirtualModel: "code", Candidates: []string{"openai/model"}}}}
	oldSigned, err := oldSigner.Sign(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	newSigned, err := newSigner.Sign(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	if err := ring.Verify(context.Background(), oldSigned); err != nil {
		t.Fatal(err)
	}
	if err := ring.Verify(context.Background(), newSigned); err != nil {
		t.Fatal(err)
	}
	newSigned.SigningKeyID = "unknown"
	if err := ring.Verify(context.Background(), newSigned); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestEd25519KeyRingLegacyKeyIDRequiresExplicitCompatibility(t *testing.T) {
	t.Parallel()
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	signer := NewEd25519Signer(private, public)
	value, err := signer.Sign(context.Background(), Snapshot{Revision: 1, GeneratedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(time.Hour).UTC(), Models: []ModelRoute{{VirtualModel: "code", Candidates: []string{"openai/model"}}}})
	if err != nil {
		t.Fatal(err)
	}
	value.SigningKeyID = ""
	payload, _ := canonicalBytes(value)
	value.Signature = ed25519.Sign(private, payload)
	strict, _ := NewEd25519KeyRing([]TrustedPublicKey{{PublicKey: public}}, false)
	if err := strict.Verify(context.Background(), value); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("strict ring accepted legacy signature: %v", err)
	}
	compatible, _ := NewEd25519KeyRing([]TrustedPublicKey{{PublicKey: public}}, true)
	if err := compatible.Verify(context.Background(), value); err != nil {
		t.Fatal(err)
	}
}

func TestParseTrustedPublicKeysSupportsJSONAndCompactForms(t *testing.T) {
	t.Parallel()
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	encoded := base64.RawURLEncoding.EncodeToString(public)
	jsonKeys, err := ParseTrustedPublicKeys(`[{"id":"next","publicKey":"` + encoded + `"}]`)
	if err != nil || len(jsonKeys) != 1 || jsonKeys[0].ID != "next" {
		t.Fatalf("JSON parse failed: %#v %v", jsonKeys, err)
	}
	compact, err := ParseTrustedPublicKeys("next=" + encoded)
	if err != nil || len(compact) != 1 || compact[0].ID != "next" {
		t.Fatalf("compact parse failed: %#v %v", compact, err)
	}
}

func TestParseTrustedPublicKeysRejectsTrailingJSONAndUnsafeKeyIDs(t *testing.T) {
	t.Parallel()
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	encoded := base64.RawURLEncoding.EncodeToString(public)
	if _, err := ParseTrustedPublicKeys(`[{"id":"next","publicKey":"` + encoded + `"}] {}`); err == nil {
		t.Fatal("expected trailing JSON rejection")
	}
	keys, err := ParseTrustedPublicKeys(`[{"id":"bad key","publicKey":"` + encoded + `"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewEd25519KeyRing(keys, false); err == nil || !errors.Is(err, core.ErrInvalidConfiguration) {
		t.Fatalf("expected unsafe key ID rejection, got %v", err)
	}
}
