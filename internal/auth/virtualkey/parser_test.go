/* llmnav/1 file
id=relaydock.auth.virtualkey.contract
role=Verify issued virtual keys round-trip through authentication and malformed key material is rejected.
search=virtual key tests|authenticate issued key|reject malformed key
stability=contract
*/
package virtualkey

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestIssuedKeyRoundTripsWithUnderscorePublicID(t *testing.T) {
	authenticator := NewMemoryAuthenticator([]byte("01234567890123456789012345678901"))
	raw, record, err := authenticator.Issue("live", "tenant", "project", []string{"models:invoke"}, nil, zeroTime())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(record.PublicID, "_") {
		t.Fatalf("fixture public ID must contain underscore: %q", record.PublicID)
	}
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.PublicID != record.PublicID || parsed.Environment != "live" {
		t.Fatalf("parsed=%+v record=%+v", parsed, record)
	}
	principal, err := authenticator.Authenticate(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if principal.ProjectID != "project" {
		t.Fatalf("principal=%+v", principal)
	}
}

func TestLegacyKeyParserUsesFixedSecretLength(t *testing.T) {
	secret := strings.Repeat("a", legacySecretLength-1) + "_"
	raw := "zg_live_vk_public_with_underscores_" + secret
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.PublicID != "vk_public_with_underscores" || parsed.Secret != secret {
		t.Fatalf("parsed=%+v", parsed)
	}
}

func zeroTime() time.Time { return time.Time{} }

func TestMalformedVirtualKeyRejected(t *testing.T) {
	for _, raw := range []string{"", "zg_live", "zg_live_.secret", "zg_live_public.short", "other_live_public.secret"} {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}
