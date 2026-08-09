package controlaccess

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/security/ssrf"
	"github.com/coreos/go-oidc/v3/oidc"
)

const (
	defaultOIDCRoleClaim    = "relaydock_role"
	defaultOIDCTenantClaim  = "relaydock_tenant_id"
	defaultOIDCProjectClaim = "relaydock_project_id"
	maximumBearerTokenBytes = 16 << 10
	maximumOIDCRoleMappings = 32
)

type OIDCConfig struct {
	Issuer       string
	Audience     string
	RoleClaim    string
	TenantClaim  string
	ProjectClaim string
	RoleMappings map[string]Role
	AllowPrivate bool
	HTTPTimeout  time.Duration
}

type oidcTokenVerifier interface {
	Verify(context.Context, string) (*oidc.IDToken, error)
}

type OIDCAuthenticator struct {
	issuer       string
	audience     string
	roleClaim    string
	tenantClaim  string
	projectClaim string
	roleMappings map[string]Role
	verifier     oidcTokenVerifier
}

func ParseOIDCRoleMappings(raw string) (map[string]Role, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var mappings map[string]Role
	if err := decoder.Decode(&mappings); err != nil {
		return nil, fmt.Errorf("decode OIDC role mappings: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("decode OIDC role mappings: multiple JSON values")
		}
		return nil, fmt.Errorf("decode OIDC role mappings: trailing data: %w", err)
	}
	if len(mappings) == 0 || len(mappings) > maximumOIDCRoleMappings {
		return nil, fmt.Errorf("OIDC role mappings must contain between 1 and %d entries", maximumOIDCRoleMappings)
	}
	result := make(map[string]Role, len(mappings))
	for source, role := range mappings {
		source = strings.TrimSpace(source)
		if source == "" || len(source) > 128 {
			return nil, fmt.Errorf("OIDC role mapping keys must contain between 1 and 128 characters")
		}
		if !knownRole(role) {
			return nil, fmt.Errorf("OIDC role mapping %q references unknown role %q", source, role)
		}
		if _, duplicate := result[source]; duplicate {
			return nil, fmt.Errorf("OIDC role mapping key %q is duplicated after trimming", source)
		}
		result[source] = role
	}
	return result, nil
}

func NewOIDCAuthenticator(ctx context.Context, config OIDCConfig, client *http.Client) (*OIDCAuthenticator, error) {
	config, err := normalizeOIDCConfig(config)
	if err != nil {
		return nil, err
	}
	validator := ssrf.Validator{AllowedSchemes: []string{"https"}, AllowPrivate: config.AllowPrivate}
	validationContext, validationCancel := context.WithTimeout(ctx, config.HTTPTimeout)
	issuerURL, err := validateOIDCEndpoint(validationContext, validator, config.Issuer, "issuer")
	validationCancel()
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = newOIDCHTTPClient(validator, config.HTTPTimeout)
	}
	clientContext := oidc.ClientContext(context.Background(), client)
	discoveryContext, cancel := context.WithTimeout(clientContext, config.HTTPTimeout)
	provider, err := oidc.NewProvider(discoveryContext, issuerURL.String())
	cancel()
	if err != nil {
		return nil, fmt.Errorf("discover OIDC issuer: %w", err)
	}
	var metadata struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, fmt.Errorf("decode OIDC discovery metadata: %w", err)
	}
	validationContext, validationCancel = context.WithTimeout(ctx, config.HTTPTimeout)
	_, err = validateOIDCEndpoint(validationContext, validator, metadata.JWKSURI, "jwks_uri")
	validationCancel()
	if err != nil {
		return nil, err
	}
	return newOIDCAuthenticator(config, provider.VerifierContext(clientContext, &oidc.Config{ClientID: config.Audience})), nil
}

func newOIDCAuthenticator(config OIDCConfig, verifier oidcTokenVerifier) *OIDCAuthenticator {
	roleMappings := make(map[string]Role, len(config.RoleMappings))
	for source, role := range config.RoleMappings {
		roleMappings[source] = role
	}
	return &OIDCAuthenticator{
		issuer: config.Issuer, audience: config.Audience, roleClaim: config.RoleClaim, tenantClaim: config.TenantClaim,
		projectClaim: config.ProjectClaim, roleMappings: roleMappings, verifier: verifier,
	}
}

func (a *OIDCAuthenticator) AuthenticateBearer(ctx context.Context, header string) (Principal, bool) {
	if a == nil || a.verifier == nil {
		return Principal{}, false
	}
	token, ok := bearerToken(header)
	if !ok || len(token) > maximumBearerTokenBytes {
		return Principal{}, false
	}
	verified, err := a.verifier.Verify(ctx, token)
	if err != nil || verified == nil || strings.TrimSpace(verified.Subject) == "" {
		return Principal{}, false
	}
	var claims map[string]json.RawMessage
	if err := verified.Claims(&claims); err != nil {
		return Principal{}, false
	}
	authorizedParty, ok := stringClaim(claims, "azp", false)
	if !ok || (len(verified.Audience) > 1 && authorizedParty == "") || (authorizedParty != "" && authorizedParty != a.audience) {
		return Principal{}, false
	}
	sourceRole, ok := stringClaim(claims, a.roleClaim, true)
	if !ok {
		return Principal{}, false
	}
	role, ok := a.roleMappings[sourceRole]
	if !ok {
		return Principal{}, false
	}
	tenantID, ok := stringClaim(claims, a.tenantClaim, false)
	if !ok {
		return Principal{}, false
	}
	projectID, ok := stringClaim(claims, a.projectClaim, false)
	if !ok {
		return Principal{}, false
	}
	principal := Principal{
		Subject: oidcSubject(a.issuer, verified.Subject), Role: role,
		TenantID: tenantID, ProjectID: projectID,
	}
	if err := principal.Validate(); err != nil {
		return Principal{}, false
	}
	return principal, true
}

func normalizeOIDCConfig(config OIDCConfig) (OIDCConfig, error) {
	config.Issuer = strings.TrimSuffix(strings.TrimSpace(config.Issuer), "/")
	config.Audience = strings.TrimSpace(config.Audience)
	if config.Issuer == "" || config.Audience == "" {
		return OIDCConfig{}, fmt.Errorf("OIDC issuer and audience are required")
	}
	if len(config.Audience) > 256 {
		return OIDCConfig{}, fmt.Errorf("OIDC audience exceeds 256 characters")
	}
	if config.HTTPTimeout <= 0 {
		config.HTTPTimeout = 10 * time.Second
	}
	if config.HTTPTimeout > time.Minute {
		return OIDCConfig{}, fmt.Errorf("OIDC HTTP timeout must not exceed one minute")
	}
	if len(config.RoleMappings) == 0 || len(config.RoleMappings) > maximumOIDCRoleMappings {
		return OIDCConfig{}, fmt.Errorf("OIDC role mappings must contain between 1 and %d entries", maximumOIDCRoleMappings)
	}
	claims := []*string{&config.RoleClaim, &config.TenantClaim, &config.ProjectClaim}
	defaults := []string{defaultOIDCRoleClaim, defaultOIDCTenantClaim, defaultOIDCProjectClaim}
	seen := make(map[string]struct{}, len(claims))
	for index, claim := range claims {
		*claim = strings.TrimSpace(*claim)
		if *claim == "" {
			*claim = defaults[index]
		}
		if !validClaimName(*claim) {
			return OIDCConfig{}, fmt.Errorf("OIDC claim name %q is invalid", *claim)
		}
		if _, duplicate := seen[*claim]; duplicate {
			return OIDCConfig{}, fmt.Errorf("OIDC claim names must be distinct")
		}
		seen[*claim] = struct{}{}
	}
	for source, role := range config.RoleMappings {
		if strings.TrimSpace(source) != source || source == "" || len(source) > 128 || !knownRole(role) {
			return OIDCConfig{}, fmt.Errorf("invalid OIDC role mapping %q", source)
		}
	}
	return config, nil
}

func validClaimName(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if !(char == '_' || char == '-' || char == '.' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9') {
			return false
		}
	}
	switch value {
	case "iss", "sub", "aud", "exp", "nbf", "iat", "nonce", "azp", "scope":
		return false
	default:
		return true
	}
}

func validateOIDCEndpoint(ctx context.Context, validator ssrf.Validator, raw, name string) (*url.URL, error) {
	target, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || target == nil || !target.IsAbs() {
		return nil, fmt.Errorf("OIDC %s must be an absolute HTTPS URL", name)
	}
	if target.RawQuery != "" || target.Fragment != "" {
		return nil, fmt.Errorf("OIDC %s must not contain a query or fragment", name)
	}
	if err := validator.Validate(ctx, target); err != nil {
		return nil, fmt.Errorf("validate OIDC %s: %w", name, err)
	}
	return target, nil
}

func newOIDCHTTPClient(validator ssrf.Validator, timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("parse OIDC upstream address: %w", err)
		}
		addresses, err := validator.ResolveAndValidate(ctx, host)
		if err != nil {
			return nil, err
		}
		var failures []error
		for _, candidate := range addresses {
			connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
			if err == nil {
				return connection, nil
			}
			failures = append(failures, err)
		}
		return nil, errors.Join(failures...)
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("OIDC redirect limit exceeded")
			}
			if _, err := validateOIDCEndpoint(request.Context(), validator, request.URL.String(), "redirect"); err != nil {
				return err
			}
			return nil
		},
	}
}

func stringClaim(claims map[string]json.RawMessage, name string, required bool) (string, bool) {
	raw, exists := claims[name]
	if !exists {
		return "", !required
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", !required
	}
	return value, true
}

func oidcSubject(issuer, subject string) string {
	digest := sha256.Sum256([]byte(issuer + "\x00" + subject))
	return "oidc:" + base64.RawURLEncoding.EncodeToString(digest[:])
}

func knownRole(role Role) bool {
	switch role {
	case RoleGateway, RoleViewer, RolePublisher, RoleAdmin:
		return true
	default:
		return false
	}
}
