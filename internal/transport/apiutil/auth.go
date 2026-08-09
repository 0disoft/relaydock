package apiutil

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func RequireBearer(token string, next http.Handler, publicPaths ...string) http.Handler {
	token = strings.TrimSpace(token)
	if token == "" {
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
		if !constantTimeBearer(r.Header.Get("Authorization"), token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="ai-runtime"`)
			WriteJSON(w, http.StatusUnauthorized, ErrorBody{Error: ErrorDetail{Code: "unauthorized", Message: "valid bearer token required"}})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func constantTimeBearer(header, expected string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	actual := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if len(actual) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}
