package controlaccess

import (
	"context"
	"net/http"
)

// Authenticator combines static bootstrap credentials and an optional OIDC
// verifier. Either source may establish a principal; authorization remains a
// separate control-access/v1 decision.
type Authenticator struct {
	static *StaticAuthenticator
	oidc   *OIDCAuthenticator
}

func NewAuthenticator(static *StaticAuthenticator, oidcAuthenticator *OIDCAuthenticator) *Authenticator {
	return &Authenticator{static: static, oidc: oidcAuthenticator}
}

func (a *Authenticator) Configured() bool {
	return a != nil && (a.static != nil && a.static.Configured() || a.oidc != nil)
}

func (a *Authenticator) AuthenticateBearer(ctx context.Context, header string) (Principal, bool) {
	if a == nil {
		return Principal{}, false
	}
	if a.static != nil {
		if principal, ok := a.static.AuthenticateBearer(header); ok {
			return principal, true
		}
	}
	if a.oidc != nil {
		return a.oidc.AuthenticateBearer(ctx, header)
	}
	return Principal{}, false
}

func (a *Authenticator) MiddlewareWithAudit(next http.Handler, audit AuditFunc, publicPaths ...string) http.Handler {
	public := make(map[string]struct{}, len(publicPaths))
	for _, path := range publicPaths {
		public[path] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := public[r.URL.Path]; ok {
			next.ServeHTTP(w, r)
			return
		}
		principal, ok := a.AuthenticateBearer(r.Context(), r.Header.Get("Authorization"))
		if !ok {
			writeAuthenticationRequired(w, r, audit)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}
