package scopedtoken

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

func (s *Service) Issue(ctx context.Context, claims Claims, lifetime time.Duration) (string, Claims, error) {
	if err := ctx.Err(); err != nil {
		return "", Claims{}, err
	}
	if err := s.validate(); err != nil {
		return "", Claims{}, err
	}
	if lifetime <= 0 || lifetime > s.maximumLifetime {
		return "", Claims{}, fmt.Errorf("%w: scoped token lifetime", core.ErrInvalidArgument)
	}
	claims.Subject = strings.TrimSpace(claims.Subject)
	claims.TenantID = strings.TrimSpace(claims.TenantID)
	claims.ProjectID = strings.TrimSpace(claims.ProjectID)
	claims.TokenID = strings.TrimSpace(claims.TokenID)
	if claims.Subject == "" {
		return "", Claims{}, fmt.Errorf("%w: scoped token subject", core.ErrInvalidArgument)
	}
	claims.Audience = s.audience
	claims.Scopes = normalizeScopes(claims.Scopes)
	now := s.currentTime()
	claims.IssuedAt = now.Unix()
	claims.ExpiresAt = now.Add(lifetime).Unix()
	if strings.TrimSpace(claims.TokenID) == "" {
		tokenID, err := randomTokenID()
		if err != nil {
			return "", Claims{}, err
		}
		claims.TokenID = tokenID
	}
	if err := validateClaimsShape(claims); err != nil {
		return "", Claims{}, err
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		return "", Claims{}, err
	}
	if len(raw) > maximumTokenPayloadBytes {
		return "", Claims{}, fmt.Errorf("%w: scoped token payload exceeds %d bytes", core.ErrInvalidArgument, maximumTokenPayloadBytes)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	signingInput := strings.Join([]string{tokenVersion, s.activeKeyID, payload}, ".")
	signature := base64.RawURLEncoding.EncodeToString(mac(s.keys[s.activeKeyID], signingInput))
	return signingInput + "." + signature, claims, nil
}

func (s *Service) Verify(ctx context.Context, raw string) (Claims, error) {
	if err := ctx.Err(); err != nil {
		return Claims{}, err
	}
	if err := s.validate(); err != nil {
		return Claims{}, err
	}
	parts := strings.Split(strings.TrimSpace(raw), ".")
	var payload string
	switch {
	case len(parts) == 4 && parts[0] == tokenVersion:
		if !validKeyID(parts[1]) || parts[2] == "" || parts[3] == "" {
			return Claims{}, core.ErrUnauthorized
		}
		secret, exists := s.keys[parts[1]]
		if !exists || !verifySignature(secret, strings.Join(parts[:3], "."), parts[3]) {
			return Claims{}, core.ErrUnauthorized
		}
		payload = parts[2]
	case len(parts) == 3 && parts[0] == legacyTokenVersion && s.allowLegacy:
		if parts[1] == "" || parts[2] == "" || !verifyLegacySignatures(s.legacySecrets, parts[0]+"."+parts[1], parts[2]) {
			return Claims{}, core.ErrUnauthorized
		}
		payload = parts[1]
	default:
		return Claims{}, core.ErrUnauthorized
	}
	claims, err := decodeClaims(payload)
	if err != nil {
		return Claims{}, err
	}
	if err := s.validateClaims(claims); err != nil {
		return Claims{}, err
	}
	claims.Scopes = normalizeScopes(claims.Scopes)
	return claims, nil
}

func decodeClaims(payload string) (Claims, error) {
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || len(raw) == 0 || len(raw) > maximumTokenPayloadBytes || base64.RawURLEncoding.EncodeToString(raw) != payload {
		return Claims{}, core.ErrUnauthorized
	}
	var claims Claims
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return Claims{}, core.ErrUnauthorized
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Claims{}, core.ErrUnauthorized
	}
	return claims, nil
}

func verifySignature(secret []byte, signingInput, encodedSignature string) bool {
	provided, err := base64.RawURLEncoding.DecodeString(encodedSignature)
	if err != nil || len(provided) != sha256.Size || base64.RawURLEncoding.EncodeToString(provided) != encodedSignature {
		return false
	}
	return subtle.ConstantTimeCompare(provided, mac(secret, signingInput)) == 1
}

func verifyLegacySignatures(secrets [][]byte, signingInput, encodedSignature string) bool {
	provided, err := base64.RawURLEncoding.DecodeString(encodedSignature)
	if err != nil || len(provided) != sha256.Size || base64.RawURLEncoding.EncodeToString(provided) != encodedSignature {
		return false
	}
	matched := 0
	for _, secret := range secrets {
		matched |= subtle.ConstantTimeCompare(provided, mac(secret, signingInput))
	}
	return matched == 1
}

func mac(secret []byte, value string) []byte {
	hash := hmac.New(sha256.New, secret)
	_, _ = hash.Write([]byte(value))
	return hash.Sum(nil)
}

func (s *Service) validateClaims(claims Claims) error {
	now := s.currentTime().Unix()
	if err := validateClaimsShape(claims); err != nil || claims.Audience != s.audience || len(normalizeScopes(claims.Scopes)) != len(claims.Scopes) {
		return core.ErrUnauthorized
	}
	if claims.IssuedAt <= 0 || claims.ExpiresAt <= claims.IssuedAt || now < claims.IssuedAt-60 || now >= claims.ExpiresAt {
		return core.ErrUnauthorized
	}
	if time.Duration(claims.ExpiresAt-claims.IssuedAt)*time.Second > s.maximumLifetime {
		return core.ErrUnauthorized
	}
	return nil
}

func (s *Service) validate() error {
	if s == nil || strings.TrimSpace(s.audience) == "" || !validKeyID(s.activeKeyID) || len(s.keys[s.activeKeyID]) < 32 {
		return fmt.Errorf("%w: scoped token service", core.ErrInvalidConfiguration)
	}
	return nil
}

func (s *Service) currentTime() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}
