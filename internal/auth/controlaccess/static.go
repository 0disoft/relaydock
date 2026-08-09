package controlaccess

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maximumCredentials = 64

type CredentialConfig struct {
	TokenSHA256 string `json:"tokenSha256"`
	Subject     string `json:"subject"`
	Role        Role   `json:"role"`
	TenantID    string `json:"tenantId,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`
}

type credential struct {
	digest    [sha256.Size]byte
	principal Principal
}

type StaticAuthenticator struct {
	credentials []credential
}

func ParseCredentialConfig(raw string) ([]CredentialConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var configs []CredentialConfig
	if err := decoder.Decode(&configs); err != nil {
		return nil, fmt.Errorf("decode control access tokens: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("decode control access tokens: multiple JSON values")
		}
		return nil, fmt.Errorf("decode control access tokens: trailing data: %w", err)
	}
	if len(configs) == 0 || len(configs) > maximumCredentials {
		return nil, fmt.Errorf("control access tokens must contain between 1 and %d credentials", maximumCredentials)
	}
	return configs, nil
}

func CredentialForToken(token string, principal Principal) CredentialConfig {
	digest := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return CredentialConfig{
		TokenSHA256: hex.EncodeToString(digest[:]),
		Subject:     principal.Subject,
		Role:        principal.Role,
		TenantID:    principal.TenantID,
		ProjectID:   principal.ProjectID,
	}
}

func NewStaticAuthenticator(configs []CredentialConfig) (*StaticAuthenticator, error) {
	if len(configs) == 0 {
		return &StaticAuthenticator{}, nil
	}
	if len(configs) > maximumCredentials {
		return nil, fmt.Errorf("control access tokens exceed maximum %d", maximumCredentials)
	}
	credentials := make([]credential, 0, len(configs))
	seen := make(map[[sha256.Size]byte]struct{}, len(configs))
	for index, config := range configs {
		digestBytes, err := hex.DecodeString(strings.TrimSpace(config.TokenSHA256))
		if err != nil || len(digestBytes) != sha256.Size {
			return nil, fmt.Errorf("control access token %d must have a 64-character SHA-256 hex digest", index)
		}
		var digest [sha256.Size]byte
		copy(digest[:], digestBytes)
		if _, exists := seen[digest]; exists {
			return nil, fmt.Errorf("control access token %d duplicates a token digest", index)
		}
		principal := Principal{Subject: config.Subject, Role: config.Role, TenantID: config.TenantID, ProjectID: config.ProjectID}
		if err := principal.Validate(); err != nil {
			return nil, fmt.Errorf("control access token %d: %w", index, err)
		}
		seen[digest] = struct{}{}
		credentials = append(credentials, credential{digest: digest, principal: principal})
	}
	return &StaticAuthenticator{credentials: credentials}, nil
}

func (a *StaticAuthenticator) Configured() bool {
	return a != nil && len(a.credentials) > 0
}

func (a *StaticAuthenticator) AuthenticateBearer(header string) (Principal, bool) {
	if a == nil {
		return Principal{}, false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return Principal{}, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return Principal{}, false
	}
	digest := sha256.Sum256([]byte(token))
	matched := -1
	for index := range a.credentials {
		if subtle.ConstantTimeCompare(digest[:], a.credentials[index].digest[:]) == 1 {
			matched = index
		}
	}
	if matched < 0 {
		return Principal{}, false
	}
	return a.credentials[matched].principal, true
}

func (a *StaticAuthenticator) Middleware(next http.Handler, publicPaths ...string) http.Handler {
	return a.MiddlewareWithAudit(next, nil, publicPaths...)
}

func (a *StaticAuthenticator) MiddlewareWithAudit(next http.Handler, audit AuditFunc, publicPaths ...string) http.Handler {
	public := make(map[string]struct{}, len(publicPaths))
	for _, path := range publicPaths {
		public[path] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := public[r.URL.Path]; ok {
			next.ServeHTTP(w, r)
			return
		}
		principal, ok := a.AuthenticateBearer(r.Header.Get("Authorization"))
		if !ok {
			if audit != nil {
				audit(r.Context(), AuditEvent{
					Action: ActionAuthenticate, Allowed: false, Reason: "authentication_required", PolicyVersion: PolicyVersion,
				})
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="relaydock-control"`)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"authentication_required","message":"valid control-plane bearer token required"}}`))
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}
