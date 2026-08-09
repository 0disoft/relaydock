package httpgateway

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/0disoft/relaydock/internal/auth/authorization"
	"github.com/0disoft/relaydock/internal/auth/virtualkey"
	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/defaults"
	"github.com/0disoft/relaydock/internal/transport/apiutil"
	"github.com/0disoft/relaydock/internal/transport/health"
)

func NewHandler() http.Handler {
	return NewHandlerWithOptions(HandlerOptions{})
}

func NewHandlerWithOptions(options HandlerOptions) http.Handler {
	if options.Compiler == nil {
		options.Compiler = defaults.Compiler()
	}
	if options.Processor == nil {
		options.Processor = EchoProcessor{}
	}
	if options.MaximumBodyBytes <= 0 {
		options.MaximumBodyBytes = apiutil.DefaultMaximumBodyBytes
	}
	if len(options.Models) == 0 {
		options.Models = []string{"local/echo"}
	}
	if strings.TrimSpace(options.Mode) == "" {
		options.Mode = "local-development"
	}

	api := &api{options: options}
	mux := http.NewServeMux()
	probe := health.Handler{Ready: func() bool {
		if api.options.Compiler == nil || api.options.Processor == nil {
			return false
		}
		return api.options.Ready == nil || api.options.Ready()
	}}
	mux.HandleFunc("GET /healthz", probe.Health)
	mux.HandleFunc("GET /readyz", probe.Readiness)
	mux.HandleFunc("GET /v1/models", api.models)
	if options.RuntimeStatus != nil {
		mux.HandleFunc("GET /v1/runtime/status", api.runtimeStatus)
	}
	mux.HandleFunc("POST /v1/responses", api.handle(canonical.ProtocolOpenAIResponses))
	mux.HandleFunc("POST /v1/chat/completions", api.handle(canonical.ProtocolOpenAIChat))
	mux.HandleFunc("POST /v1/messages", api.handle(canonical.ProtocolAnthropicMessages))
	mux.HandleFunc("POST /v1beta/models/", api.gemini)
	return requestMetadata(options.MaximumBodyBytes, options.Mode, mux)
}

type api struct{ options HandlerOptions }

func (a *api) models(w http.ResponseWriter, r *http.Request) {
	configuredModels := a.options.Models
	if a.options.ModelSource != nil {
		configuredModels = a.options.ModelSource.Models()
	}
	models := make([]map[string]any, 0, len(configuredModels))
	principal, restricted := virtualkey.PrincipalFromContext(r.Context())
	for _, model := range configuredModels {
		if restricted && principal.Authorize(authorization.ScopeModelsInvoke, model) != nil {
			continue
		}
		models = append(models, map[string]any{
			"id": model, "object": "model", "owned_by": "ai-runtime-gateway",
		})
	}
	apiutil.WriteJSON(w, http.StatusOK, map[string]any{"object": "list", "data": models})
}

func (a *api) runtimeStatus(w http.ResponseWriter, r *http.Request) {
	status, err := a.options.RuntimeStatus(r.Context())
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	if status == nil {
		status = map[string]any{}
	}
	status["mode"] = a.options.Mode
	status["ready"] = a.options.Ready == nil || a.options.Ready()
	apiutil.WriteJSON(w, http.StatusOK, status)
}

func (a *api) handle(protocol canonical.Protocol) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.process(w, r, protocol, "", false)
	}
}

func (a *api) gemini(w http.ResponseWriter, r *http.Request) {
	operation := strings.TrimPrefix(r.URL.Path, "/v1beta/models/")
	model, action, ok := strings.Cut(operation, ":")
	if !ok || strings.TrimSpace(model) == "" {
		apiutil.WriteError(w, fmt.Errorf("%w: Gemini model path", core.ErrInvalidArgument))
		return
	}
	streaming := action == "streamGenerateContent" || r.URL.Query().Get("alt") == "sse"
	if action != "generateContent" && action != "streamGenerateContent" {
		apiutil.WriteError(w, fmt.Errorf("%w: Gemini operation %q", core.ErrNotFound, action))
		return
	}
	a.process(w, r, canonical.ProtocolGeminiGenerate, model, streaming)
}
