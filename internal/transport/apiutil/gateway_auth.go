package apiutil

import (
	"context"
	"net/http"
	"strings"

	"github.com/0disoft/relaydock/internal/auth/authorization"
	"github.com/0disoft/relaydock/internal/auth/virtualkey"
)

// RequireGatewayAuthentication accepts either the private operator bearer or
// a scoped virtual key. Operator access receives only the model invocation
// scope here; control-plane administration remains on a separate service.
func RequireGatewayAuthentication(staticToken string, authenticator virtualkey.Authenticator, next http.Handler, publicPaths ...string) http.Handler {
	staticToken = strings.TrimSpace(staticToken)
	if staticToken == "" && authenticator == nil {
		return next
	}
	public := make(map[string]struct{}, len(publicPaths))
	for _, path := range publicPaths {
		public[path] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := public[r.URL.Path]; ok {
			next.ServeHTTP(w, r)
			return
		}
		header := r.Header.Get("Authorization")
		if staticToken != "" && constantTimeBearer(header, staticToken) {
			principal := virtualkey.Principal{
				VirtualKeyID: "operator-bearer",
				Scopes:       []string{authorization.ScopeModelsInvoke},
			}
			next.ServeHTTP(w, r.WithContext(virtualkey.WithPrincipal(r.Context(), principal)))
			return
		}
		if authenticator != nil {
			raw, ok := bearerValue(header)
			if ok {
				principal, err := authenticator.Authenticate(r.Context(), raw)
				if err == nil && principal.Authorize(authorization.ScopeModelsInvoke, "") == nil {
					next.ServeHTTP(w, r.WithContext(virtualkey.WithPrincipal(r.Context(), principal)))
					return
				}
			}
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="relaydock"`)
		WriteJSON(w, http.StatusUnauthorized, ErrorBody{Error: ErrorDetail{Code: "unauthorized", Message: "valid gateway API key required"}})
	})
}

func bearerValue(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	value := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return value, value != ""
}

// Principal returns the authenticated gateway principal to handlers that need
// project scope or model restrictions.
func Principal(ctx context.Context) (virtualkey.Principal, bool) {
	return virtualkey.PrincipalFromContext(ctx)
}
