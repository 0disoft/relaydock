package snapshot

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"
)

func TestParseSigningPrivateKeyAcceptsSeedAndPrivateKey(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	encodings := []string{
		base64.RawURLEncoding.EncodeToString(seed),
		base64.StdEncoding.EncodeToString(privateKey),
	}
	for _, encoded := range encodings {
		signer, err := ParseSigningPrivateKey(encoded)
		if err != nil {
			t.Fatalf("parse key: %v", err)
		}
		value := Snapshot{
			Revision:    1,
			GeneratedAt: time.Unix(1_700_000_000, 0).UTC(),
			ExpiresAt:   time.Unix(1_700_003_600, 0).UTC(),
			Models: []ModelRoute{{
				VirtualModel: "local/echo",
				Candidates:   []string{"local/echo"},
			}},
		}
		signed, err := signer.Sign(context.Background(), value)
		if err != nil {
			t.Fatalf("sign with parsed key: %v", err)
		}
		if err := signer.Verify(context.Background(), signed); err != nil {
			t.Fatalf("verify with parsed key: %v", err)
		}
	}
}

func TestParseSigningPrivateKeyRejectsMalformedInput(t *testing.T) {
	for _, value := range []string{"", "not base64", base64.RawURLEncoding.EncodeToString([]byte("too short"))} {
		if _, err := ParseSigningPrivateKey(value); err == nil {
			t.Fatalf("expected malformed key %q to fail", value)
		}
	}
}
