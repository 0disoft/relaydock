package experthttp

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	expertapp "github.com/your-org/ai-runtime-gateway/internal/expert/app"
	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	expertpolicy "github.com/your-org/ai-runtime-gateway/internal/expert/policy"
	"github.com/your-org/ai-runtime-gateway/internal/expert/resultcontract"
	"github.com/your-org/ai-runtime-gateway/internal/transport/apiutil"
	"github.com/your-org/ai-runtime-gateway/internal/transport/health"
)

type CreateRequest struct {
	TenantID         string                `json:"tenantId,omitempty"`
	ProjectID        string                `json:"projectId,omitempty"`
	RepositoryRoot   string                `json:"repositoryRoot"`
	Objective        string                `json:"objective"`
	TaskType         string                `json:"taskType,omitempty"`
	Route            consultation.Route    `json:"route,omitempty"`
	CandidatePaths   []string              `json:"candidatePaths,omitempty"`
	SuccessCriteria  []string              `json:"successCriteria,omitempty"`
	Attempts         []contextpack.Attempt `json:"attempts,omitempty"`
	OpenQuestions    []string              `json:"openQuestions,omitempty"`
	MaximumBytes     int64                 `json:"maximumBytes,omitempty"`
	MaximumCostMinor int64                 `json:"maximumCostMinor,omitempty"`
	IdempotencyKey   string                `json:"idempotencyKey,omitempty"`
	TTLSeconds       int64                 `json:"ttlSeconds,omitempty"`
	AutoApprove      bool                  `json:"autoApprove,omitempty"`
	DelegationDepth  int                   `json:"delegationDepth,omitempty"`
}

type CreateResponse struct {
	Consultation consultation.Consultation `json:"consultation"`
	ContextPack  contextpack.Pack          `json:"contextPack"`
}

type SubmitResultRequest struct {
	Result           resultcontract.Result `json:"result"`
	ModelAttestation string                `json:"modelAttestation"`
}

type ConsultationResponse struct {
	Consultation consultation.Consultation `json:"consultation"`
	ContextPack  *contextpack.Pack         `json:"contextPack,omitempty"`
	Result       *resultcontract.Result    `json:"result,omitempty"`
	ResultRecord *resultcontract.Record    `json:"resultRecord,omitempty"`
}

type API struct{ app *expertapp.App }

func NewHandler() http.Handler {
	application, err := expertapp.NewInMemory()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { apiutil.WriteError(w, err) })
	}
	return NewHandlerWithApp(application)
}

func NewHandlerWithApp(application *expertapp.App) http.Handler {
	api := &API{app: application}
	mux := http.NewServeMux()
	probe := health.Handler{Ready: func() bool { return application != nil && application.Consultations != nil }}
	mux.HandleFunc("GET /healthz", probe.Health)
	mux.HandleFunc("GET /readyz", probe.Readiness)
	mux.HandleFunc("POST /v1/context-packs/preview", api.preview)
	mux.HandleFunc("POST /v1/context-packs", api.buildContext)
	mux.HandleFunc("GET /v1/context-packs/{id}", api.getContext)
	mux.HandleFunc("GET /v1/consultations", api.list)
	mux.HandleFunc("POST /v1/consultations", api.create)
	mux.HandleFunc("GET /v1/consultations/{id}", api.get)
	mux.HandleFunc("POST /v1/consultations/{id}/approve", api.approve)
	mux.HandleFunc("POST /v1/consultations/{id}/cancel", api.cancel)
	mux.HandleFunc("POST /v1/consultations/{id}/result", api.submitResult)
	return mux
}

func (a *API) preview(w http.ResponseWriter, r *http.Request) {
	var request contextpack.BuildRequest
	if err := apiutil.ReadJSON(w, r, apiutil.DefaultMaximumBodyBytes, &request); err != nil {
		apiutil.WriteError(w, err)
		return
	}
	pack, err := a.app.PreviewContext(r.Context(), request)
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, pack)
}

func (a *API) buildContext(w http.ResponseWriter, r *http.Request) {
	var request contextpack.BuildRequest
	if err := apiutil.ReadJSON(w, r, apiutil.DefaultMaximumBodyBytes, &request); err != nil {
		apiutil.WriteError(w, err)
		return
	}
	pack, err := a.app.BuildContext(r.Context(), request)
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	apiutil.WriteJSON(w, http.StatusCreated, pack)
}

func (a *API) getContext(w http.ResponseWriter, r *http.Request) {
	pack, err := a.app.Packs.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, pack)
}

func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var request CreateRequest
	if err := apiutil.ReadJSON(w, r, apiutil.DefaultMaximumBodyBytes, &request); err != nil {
		apiutil.WriteError(w, err)
		return
	}
	ttl := time.Duration(request.TTLSeconds) * time.Second
	if ttl < 0 || ttl > 7*24*time.Hour {
		apiutil.WriteError(w, fmt.Errorf("%w: ttlSeconds must be between 0 and 604800", core.ErrInvalidArgument))
		return
	}
	if err := expertpolicy.NewDelegationGuard(1).Check(request.DelegationDepth); err != nil {
		apiutil.WriteError(w, err)
		return
	}
	created, pack, err := a.app.CreateWithContext(r.Context(), contextpack.BuildRequest{
		TenantID:        request.TenantID,
		ProjectID:       request.ProjectID,
		RepositoryRoot:  request.RepositoryRoot,
		Objective:       request.Objective,
		SuccessCriteria: request.SuccessCriteria,
		CandidatePaths:  request.CandidatePaths,
		MaximumBytes:    request.MaximumBytes,
		Attempts:        request.Attempts,
		OpenQuestions:   request.OpenQuestions,
	}, consultation.CreateCommand{
		TenantID:         request.TenantID,
		ProjectID:        request.ProjectID,
		Objective:        request.Objective,
		TaskType:         request.TaskType,
		Route:            request.Route,
		MaximumCostMinor: request.MaximumCostMinor,
		IdempotencyKey:   request.IdempotencyKey,
		TTL:              ttl,
	})
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	if request.AutoApprove && created.State == consultation.StateApprovalPending {
		created, err = a.app.Consultations.Approve(r.Context(), created.ID)
		if err != nil {
			apiutil.WriteError(w, err)
			return
		}
	}
	apiutil.WriteJSON(w, http.StatusCreated, CreateResponse{Consultation: created, ContextPack: pack})
}

func (a *API) list(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	items, err := a.app.Consultations.List(r.Context(), limit)
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (a *API) get(w http.ResponseWriter, r *http.Request) {
	item, err := a.app.Consultations.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	response := ConsultationResponse{Consultation: item}
	if item.ContextPackID != "" {
		if pack, packErr := a.app.Packs.Get(r.Context(), item.ContextPackID); packErr == nil {
			response.ContextPack = &pack
		}
	}
	if item.ResultID != "" {
		if records, ok := a.app.Results.(resultcontract.RecordStore); ok {
			if record, recordErr := records.GetRecord(r.Context(), item.ResultID); recordErr == nil {
				response.ResultRecord = &record
				response.Result = &record.Result
			}
		} else if result, resultErr := a.app.Results.Get(r.Context(), item.ResultID); resultErr == nil {
			response.Result = &result
		}
	}
	apiutil.WriteJSON(w, http.StatusOK, response)
}

func (a *API) approve(w http.ResponseWriter, r *http.Request) {
	item, err := a.app.Consultations.Approve(r.Context(), r.PathValue("id"))
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, item)
}

func (a *API) cancel(w http.ResponseWriter, r *http.Request) {
	item, err := a.app.Consultations.Cancel(r.Context(), r.PathValue("id"))
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, item)
}

func (a *API) submitResult(w http.ResponseWriter, r *http.Request) {
	var request SubmitResultRequest
	if err := apiutil.ReadJSON(w, r, apiutil.DefaultMaximumBodyBytes, &request); err != nil {
		apiutil.WriteError(w, err)
		return
	}
	if strings.TrimSpace(request.ModelAttestation) == "" {
		apiutil.WriteError(w, fmt.Errorf("%w: modelAttestation is required", core.ErrInvalidArgument))
		return
	}
	item, resultID, err := a.app.StoreResultAttested(r.Context(), r.PathValue("id"), request.ModelAttestation, request.Result)
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, map[string]any{"consultation": item, "resultId": resultID, "modelAttestation": request.ModelAttestation})
}
