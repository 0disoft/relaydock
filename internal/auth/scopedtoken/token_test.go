package scopedtoken

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

func TestScopedTokenRoundTripAndScopes(t *testing.T) {
	service, err := New([]byte(strings.Repeat("s", 32)), "expert-mcp")
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixed }
	raw, issued, err := service.Issue(context.Background(), Claims{
		Subject: "chatgpt-web",
		Scopes:  []string{"consultations:read", "consultations:read"},
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := service.Verify(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Subject != issued.Subject || verified.TokenID == "" || !verified.HasScope("consultations:read") {
		t.Fatalf("verified claims = %+v", verified)
	}
	if verified.HasScope("consultations:answer") {
		t.Fatal("read token unexpectedly has answer scope")
	}
}

func TestScopedTokenRejectsTamperingExpiryAndWrongAudience(t *testing.T) {
	service, _ := New([]byte(strings.Repeat("a", 32)), "expert-mcp")
	fixed := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixed }
	raw, _, err := service.Issue(context.Background(), Claims{Subject: "user", Scopes: []string{"consultations:read"}}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tampered := raw[:len(raw)-1] + "x"
	if _, err := service.Verify(context.Background(), tampered); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("tampered error = %v", err)
	}
	service.now = func() time.Time { return fixed.Add(2 * time.Minute) }
	if _, err := service.Verify(context.Background(), raw); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("expired error = %v", err)
	}
	other, _ := New([]byte(strings.Repeat("a", 32)), "different-audience")
	other.now = func() time.Time { return fixed }
	if _, err := other.Verify(context.Background(), raw); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("audience error = %v", err)
	}
}

func TestScopedTokenKeyRotationAndLegacyOverlap(t *testing.T) {
	oldSecret := []byte(strings.Repeat("o", 32))
	newSecret := []byte(strings.Repeat("n", 32))
	oldService, err := NewKeyRing("old", oldSecret, nil, "expert-mcp")
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	oldService.now = func() time.Time { return fixed }
	oldToken, _, err := oldService.Issue(context.Background(), Claims{Subject: "old-client", Scopes: []string{"consultations:read"}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := NewKeyRing("new", newSecret, map[string][]byte{"old": oldSecret}, "expert-mcp")
	if err != nil {
		t.Fatal(err)
	}
	rotated.now = func() time.Time { return fixed }
	if _, err := rotated.Verify(context.Background(), oldToken); err != nil {
		t.Fatalf("rotated verifier rejected old v2 token: %v", err)
	}
	newToken, _, err := rotated.Issue(context.Background(), Claims{Subject: "new-client", Scopes: []string{"consultations:read"}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(newToken, "argt2.new.") {
		t.Fatalf("new token does not identify active key: %s", newToken)
	}
	if _, err := oldService.Verify(context.Background(), newToken); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("old verifier unexpectedly accepted new token: %v", err)
	}

	legacyClaims := Claims{Subject: "legacy", Audience: "expert-mcp", Scopes: []string{"consultations:read"}, TokenID: "legacy-id", IssuedAt: fixed.Unix(), ExpiresAt: fixed.Add(time.Hour).Unix()}
	payloadRaw, err := json.Marshal(legacyClaims)
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadRaw)
	input := legacyTokenVersion + "." + payload
	legacyToken := input + "." + base64.RawURLEncoding.EncodeToString(mac(oldSecret, input))
	if _, err := rotated.Verify(context.Background(), legacyToken); err != nil {
		t.Fatalf("rotation overlap rejected legacy token: %v", err)
	}
	if _, err := rotated.WithLegacyVerification(false).Verify(context.Background(), legacyToken); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("legacy-disabled verifier accepted argt1 token: %v", err)
	}
}

func TestScopedTokenRejectsUnknownAndMalformedKeyIDs(t *testing.T) {
	service, err := NewKeyRing("active", []byte(strings.Repeat("a", 32)), nil, "expert-mcp")
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixed }
	raw, _, err := service.Issue(context.Background(), Claims{Subject: "user", Scopes: []string{"consultations:read"}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	unknown := strings.Replace(raw, "argt2.active.", "argt2.unknown.", 1)
	if _, err := service.Verify(context.Background(), unknown); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("unknown key error = %v", err)
	}
	malformed := strings.Replace(raw, "argt2.active.", "argt2.bad.key.", 1)
	if _, err := service.Verify(context.Background(), malformed); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("malformed key error = %v", err)
	}
}

func TestParseBase64KeyMapRejectsTrailingJSONAndNormalizedCollisions(t *testing.T) {
	secret := base64.RawStdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	if _, err := ParseBase64KeyMap(`{"old":"` + secret + `"} {}`); err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("expected trailing JSON rejection, got %v", err)
	}
	if _, err := ParseBase64KeyMap(`{"old":"` + secret + `"," old ":"` + secret + `"}`); err == nil || !strings.Contains(err.Error(), "duplicate normalized") {
		t.Fatalf("expected normalized key collision rejection, got %v", err)
	}
}

func TestScopedTokenRejectsOversizedClaimsBeforeSigning(t *testing.T) {
	service, err := NewKeyRing("active", []byte(strings.Repeat("a", 32)), nil, "expert-mcp")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.Issue(context.Background(), Claims{
		Subject: strings.Repeat("s", maximumSubjectBytes+1),
		Scopes:  []string{"consultations:read"},
	}, time.Hour)
	if !errors.Is(err, core.ErrInvalidArgument) {
		t.Fatalf("expected subject bound rejection, got %v", err)
	}

	scopes := make([]string, maximumScopeCount+1)
	for index := range scopes {
		scopes[index] = fmt.Sprintf("scope:%d", index)
	}
	_, _, err = service.Issue(context.Background(), Claims{Subject: "user", Scopes: scopes}, time.Hour)
	if !errors.Is(err, core.ErrInvalidArgument) {
		t.Fatalf("expected scope count rejection, got %v", err)
	}
}
