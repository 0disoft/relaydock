package localruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/your-org/ai-runtime-gateway/internal/appdirs"
	"github.com/your-org/ai-runtime-gateway/internal/core"
	expertapp "github.com/your-org/ai-runtime-gateway/internal/expert/app"
	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	expertpolicy "github.com/your-org/ai-runtime-gateway/internal/expert/policy"
	"github.com/your-org/ai-runtime-gateway/internal/localipc"
	"github.com/your-org/ai-runtime-gateway/internal/mcpcontract"
)

type Runtime struct {
	App                   *expertapp.App
	DefaultRepositoryRoot string
}

// NewPersistent constructs a local runtime whose expert state survives process
// restarts. The caller may provide an explicit data root for portable installs
// and tests; an empty root resolves to the operating system user config path.
func NewPersistent(dataRoot, defaultRepositoryRoot string) (*Runtime, error) {
	statePath, err := appdirs.ExpertStatePath(dataRoot)
	if err != nil {
		return nil, err
	}
	application, err := expertapp.NewLocal(statePath)
	if err != nil {
		return nil, err
	}
	return New(application, defaultRepositoryRoot)
}

func New(application *expertapp.App, defaultRepositoryRoot string) (*Runtime, error) {
	if application == nil {
		var err error
		application, err = expertapp.NewInMemory()
		if err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(defaultRepositoryRoot) == "" {
		defaultRepositoryRoot, _ = os.Getwd()
	}
	if defaultRepositoryRoot != "" {
		if absolute, err := filepath.Abs(defaultRepositoryRoot); err == nil {
			defaultRepositoryRoot = absolute
		}
	}
	return &Runtime{App: application, DefaultRepositoryRoot: defaultRepositoryRoot}, nil
}

func (r *Runtime) Handler() localipc.Handler {
	mux := localipc.NewMux()
	mux.Register("context.preview", r.preview)
	mux.Register("consultation.create", r.create)
	mux.Register("consultation.get", r.get)
	mux.Register("consultation.cancel", r.cancel)
	mux.Register("runtime.status", r.status)
	return mux
}

func (r *Runtime) preview(ctx context.Context, payload json.RawMessage) (any, error) {
	var input mcpcontract.ContextPreviewInput
	if err := decode(payload, &input); err != nil {
		return nil, err
	}
	pack, err := r.App.PreviewContext(ctx, contextpack.BuildRequest{
		RepositoryRoot: r.repositoryRoot(input.RepositoryRoot),
		Objective:      input.Objective,
		CandidatePaths: input.CandidatePaths,
	})
	if err != nil {
		return nil, err
	}
	return mcpcontract.ContextPreviewOutput{
		ContextPackID:    pack.ID,
		IncludedFiles:    len(pack.Evidence),
		ExcludedFindings: pack.RedactionReport.RemovedFiles + pack.RedactionReport.RemovedSegments,
	}, nil
}

func (r *Runtime) create(ctx context.Context, payload json.RawMessage) (any, error) {
	var input mcpcontract.ConsultationCreateInput
	if err := decode(payload, &input); err != nil {
		return nil, err
	}
	if err := expertpolicy.NewDelegationGuard(1).Check(input.DelegationDepth); err != nil {
		return nil, err
	}
	item, _, err := r.App.CreateWithContext(ctx, contextpack.BuildRequest{
		RepositoryRoot: r.repositoryRoot(input.RepositoryRoot),
		Objective:      input.Objective,
		CandidatePaths: input.CandidatePaths,
	}, consultation.CreateCommand{
		Objective:        input.Objective,
		TaskType:         input.TaskType,
		Route:            consultation.RouteOpenAIAPIPro,
		MaximumCostMinor: input.MaximumCostMinor,
	})
	if err != nil {
		return nil, err
	}
	return mcpcontract.ConsultationCreateOutput{ConsultationID: item.ID, State: string(item.State)}, nil
}

func (r *Runtime) get(ctx context.Context, payload json.RawMessage) (any, error) {
	var input mcpcontract.ConsultationGetInput
	if err := decode(payload, &input); err != nil {
		return nil, err
	}
	item, err := r.App.Consultations.Get(ctx, input.ConsultationID)
	if err != nil {
		return nil, err
	}
	output := mcpcontract.ConsultationGetOutput{ConsultationID: item.ID, State: string(item.State)}
	if item.ResultID != "" {
		result, resultErr := r.App.Results.Get(ctx, item.ResultID)
		if resultErr != nil {
			return nil, resultErr
		}
		output.Decision = result.Decision
		output.Result = &result
	}
	return output, nil
}

func (r *Runtime) cancel(ctx context.Context, payload json.RawMessage) (any, error) {
	var input mcpcontract.ConsultationCancelInput
	if err := decode(payload, &input); err != nil {
		return nil, err
	}
	item, err := r.App.Consultations.Cancel(ctx, input.ConsultationID)
	if err != nil {
		return nil, err
	}
	return mcpcontract.ConsultationCancelOutput{ConsultationID: item.ID, State: string(item.State)}, nil
}

func (r *Runtime) status(ctx context.Context, _ json.RawMessage) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items, err := r.App.Consultations.List(ctx, 500)
	if err != nil {
		return nil, err
	}
	active := 0
	for _, item := range items {
		if !item.Terminal() {
			active++
		}
	}
	return map[string]any{"ready": true, "activeConsultations": active, "repositoryRoot": r.DefaultRepositoryRoot}, nil
}

func (r *Runtime) repositoryRoot(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return r.DefaultRepositoryRoot
}

func decode(payload json.RawMessage, destination any) error {
	if len(payload) == 0 || string(payload) == "null" {
		return fmt.Errorf("%w: IPC payload is required", core.ErrInvalidArgument)
	}
	if err := json.Unmarshal(payload, destination); err != nil {
		return fmt.Errorf("%w: malformed IPC payload: %v", core.ErrInvalidArgument, err)
	}
	return nil
}
