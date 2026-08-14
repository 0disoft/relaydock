package mcpremote

/* llmnav/1 module
id=relaydock.mcp.remote
role=Expose tenant-scoped remote expert tools behind bearer authentication, host, origin, body-size, and per-tool scope checks.
owns=remote MCP tool registry|remote MCP authentication|MCP request exposure limits
excludes=consultation domain logic|token issuance
search=remote MCP server|consultation tool scopes|authenticate MCP request
invariant=Reading and answering consultations require distinct explicit scopes.
invariant=Untrusted requests are bounded before reaching MCP handlers.
stability=architecture
*/

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/0disoft/relaydock/internal/auth/mcpscope"
	"github.com/0disoft/relaydock/internal/auth/scopedtoken"
	"github.com/0disoft/relaydock/internal/core"
)

func NewHandler(backend Backend, version string) http.Handler {
	return NewHandlerWithOptions(backend, version, HandlerOptions{})
}

func NewHandlerWithOptions(backend Backend, version string, options HandlerOptions) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ai-runtime-expert-remote",
		Version: version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "consultation_get",
		Description: "Read an approved consultation and its redacted ContextPack.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ConsultationGetInput) (*mcp.CallToolResult, ConsultationGetOutput, error) {
		if err := requireRemoteScope(ctx, ScopeConsultationsRead); err != nil {
			return nil, ConsultationGetOutput{}, err
		}
		output, err := backend.GetConsultation(ctx, input)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "consultation_submit_result",
		Description: "Submit a structured expert review to an existing consultation.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input SubmitResultInput) (*mcp.CallToolResult, SubmitResultOutput, error) {
		if err := requireRemoteScope(ctx, ScopeConsultationsAnswer); err != nil {
			return nil, SubmitResultOutput{}, err
		}
		output, err := backend.SubmitResult(ctx, input)
		return nil, output, err
	})

	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true})

	if options.MaximumBodyBytes <= 0 {
		options.MaximumBodyBytes = 4 << 20
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host, options.AllowedHosts) || !originAllowed(r.Header.Get("Origin"), options.AllowedOrigins) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		claims, err := authenticateRequest(r, options)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="expert-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r = r.WithContext(scopedtoken.WithClaims(r.Context(), claims))
		r.Body = http.MaxBytesReader(w, r.Body, options.MaximumBodyBytes)
		if options.Logger != nil {
			options.Logger.InfoContext(r.Context(), "remote MCP request", "method", r.Method, "host", r.Host, "origin", r.Header.Get("Origin"))
		}
		mcpHandler.ServeHTTP(w, r)
	})
}

func validBearer(header, expected string) bool {
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

func hostAllowed(host string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	host = normalizeHost(host)
	for _, candidate := range allowed {
		if host == normalizeHost(candidate) {
			return true
		}
	}
	return false
}

func normalizeHost(value string) string {
	value = strings.TrimSpace(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	return strings.ToLower(strings.Trim(value, "[]"))
}

func originAllowed(origin string, allowed []string) bool {
	if origin == "" || len(allowed) == 0 {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	normalized := strings.ToLower(parsed.Scheme + "://" + parsed.Host)
	for _, candidate := range allowed {
		if normalized == strings.ToLower(strings.TrimRight(strings.TrimSpace(candidate), "/")) {
			return true
		}
	}
	return false
}

func authenticateRequest(r *http.Request, options HandlerOptions) (scopedtoken.Claims, error) {
	header := r.Header.Get("Authorization")
	if options.TokenVerifier != nil {
		raw, ok := bearerToken(header)
		if ok {
			claims, err := options.TokenVerifier.Verify(r.Context(), raw)
			if err == nil {
				return claims, nil
			}
		}
	}
	if options.BearerToken != "" && validBearer(header, options.BearerToken) {
		return scopedtoken.Claims{
			Subject: "legacy-static-token",
			Scopes:  []string{ScopeConsultationsRead, ScopeConsultationsAnswer},
		}, nil
	}
	if options.TokenVerifier == nil && options.BearerToken == "" {
		return scopedtoken.Claims{Subject: "local-unsecured", Scopes: []string{"*"}}, nil
	}
	return scopedtoken.Claims{}, core.ErrUnauthorized
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	value := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return value, value != ""
}

func requireRemoteScope(ctx context.Context, required string) error {
	claims, ok := scopedtoken.FromContext(ctx)
	if !ok {
		return core.ErrUnauthorized
	}
	if !mcpscope.Allows(claims.Scopes, required) {
		return core.ErrForbidden
	}
	return nil
}
