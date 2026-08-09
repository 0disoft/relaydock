package desktopwails

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	expertapp "github.com/0disoft/relaydock/internal/expert/app"
	"github.com/0disoft/relaydock/internal/expert/consultation"
	"github.com/0disoft/relaydock/internal/expert/contextpack"
	"github.com/0disoft/relaydock/internal/expert/resultcontract"
	"github.com/0disoft/relaydock/internal/expert/routes/openaiapi"
	"github.com/0disoft/relaydock/internal/expert/routes/webhandoff"
)

type ConsultationCreateInput struct {
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
	AutoApprove      bool                  `json:"autoApprove,omitempty"`
}

type ConsultationCreateOutput struct {
	Consultation consultation.Consultation `json:"consultation"`
	ContextPack  contextpack.Pack          `json:"contextPack"`
}

type ExpertRunInput struct {
	ConsultationID  string `json:"consultationId"`
	Model           string `json:"model,omitempty"`
	ReasoningMode   string `json:"reasoningMode,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
}

type WebHandoffOutput struct {
	ConsultationID string    `json:"consultationId"`
	ReadToken      string    `json:"readToken"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

type ConsultationService struct {
	app      *expertapp.App
	handoffs *webhandoff.Service
}

func NewConsultationService(application *expertapp.App) *ConsultationService {
	return &ConsultationService{app: application, handoffs: webhandoff.NewService(24 * time.Hour)}
}

func (s *ConsultationService) PreviewContext(request contextpack.BuildRequest) (contextpack.Pack, error) {
	ctx := context.Background()
	if err := s.validate(); err != nil {
		return contextpack.Pack{}, err
	}
	return s.app.PreviewContext(ctx, request)
}

func (s *ConsultationService) Create(input ConsultationCreateInput) (ConsultationCreateOutput, error) {
	ctx := context.Background()
	if err := s.validate(); err != nil {
		return ConsultationCreateOutput{}, err
	}
	item, pack, err := s.app.CreateWithContext(ctx, contextpack.BuildRequest{
		RepositoryRoot:  input.RepositoryRoot,
		Objective:       input.Objective,
		SuccessCriteria: input.SuccessCriteria,
		CandidatePaths:  input.CandidatePaths,
		MaximumBytes:    input.MaximumBytes,
		Attempts:        input.Attempts,
		OpenQuestions:   input.OpenQuestions,
	}, consultation.CreateCommand{
		Objective:        input.Objective,
		TaskType:         input.TaskType,
		Route:            input.Route,
		MaximumCostMinor: input.MaximumCostMinor,
	})
	if err != nil {
		return ConsultationCreateOutput{}, err
	}
	if input.AutoApprove && item.State == consultation.StateApprovalPending {
		item, err = s.app.Consultations.Approve(ctx, item.ID)
		if err != nil {
			return ConsultationCreateOutput{}, err
		}
	}
	return ConsultationCreateOutput{Consultation: item, ContextPack: pack}, nil
}

func (s *ConsultationService) Get(id string) (consultation.Consultation, error) {
	ctx := context.Background()
	if err := s.validate(); err != nil {
		return consultation.Consultation{}, err
	}
	return s.app.Consultations.Get(ctx, id)
}

func (s *ConsultationService) List(limit int) ([]consultation.Consultation, error) {
	ctx := context.Background()
	if err := s.validate(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.app.Consultations.List(ctx, limit)
}

func (s *ConsultationService) Approve(id string) (consultation.Consultation, error) {
	ctx := context.Background()
	if err := s.validate(); err != nil {
		return consultation.Consultation{}, err
	}
	return s.app.Consultations.Approve(ctx, id)
}

func (s *ConsultationService) Cancel(id string) (consultation.Consultation, error) {
	ctx := context.Background()
	if err := s.validate(); err != nil {
		return consultation.Consultation{}, err
	}
	return s.app.Consultations.Cancel(ctx, id)
}

func (s *ConsultationService) RunAPIExpert(input ExpertRunInput) (consultation.Consultation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if err := s.validate(); err != nil {
		return consultation.Consultation{}, err
	}
	item, err := s.prepareRun(ctx, input.ConsultationID)
	if err != nil {
		return consultation.Consultation{}, err
	}
	pack, err := s.app.Packs.Get(ctx, item.ContextPackID)
	if err != nil {
		return consultation.Consultation{}, err
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = strings.TrimSpace(os.Getenv("EXPERT_MODEL"))
	}
	if model == "" {
		model = "gpt-5.6-sol"
	}
	mode := strings.TrimSpace(input.ReasoningMode)
	if mode == "" {
		mode = "pro"
	}
	effort := strings.TrimSpace(input.ReasoningEffort)
	if effort == "" {
		effort = "max"
	}
	client := openaiapi.New(strings.TrimSpace(os.Getenv("OPENAI_RESPONSES_ENDPOINT")), os.Getenv("OPENAI_API_KEY"), nil)
	pricing, maximumOutputTokens, pricingErr := expertPricingFromEnvironment()
	if pricingErr != nil {
		_, _ = s.app.Repository.SetFailure(context.Background(), item.ID, pricingErr.Error())
		return consultation.Consultation{}, pricingErr
	}
	if pricing != nil {
		client = client.WithPricing(*pricing)
	}
	result, err := client.Run(ctx, pack, openaiapi.RunOptions{
		Model:               model,
		ReasoningMode:       mode,
		ReasoningEffort:     effort,
		MaximumCostMinor:    item.MaximumCostMinor,
		MaximumOutputTokens: maximumOutputTokens,
	})
	if err != nil {
		_, _ = s.app.Repository.SetFailure(context.Background(), item.ID, err.Error())
		return consultation.Consultation{}, err
	}
	attestation := fmt.Sprintf("provider=openai;model=%s;reasoning_mode=%s;reasoning_effort=%s;attestation=configured_request", model, mode, effort)
	updated, _, err := s.app.StoreResultAttested(ctx, item.ID, attestation, result)
	return updated, err
}

func (s *ConsultationService) CreateWebHandoff(id string) (WebHandoffOutput, error) {
	ctx := context.Background()
	if err := s.validate(); err != nil {
		return WebHandoffOutput{}, err
	}
	item, err := s.app.Consultations.Get(ctx, id)
	if err != nil {
		return WebHandoffOutput{}, err
	}
	if item.State == consultation.StateApprovalPending {
		item, err = s.app.Consultations.Approve(ctx, id)
		if err != nil {
			return WebHandoffOutput{}, err
		}
	}
	if item.State != consultation.StateQueued && item.State != consultation.StateRunning {
		return WebHandoffOutput{}, fmt.Errorf("%w: handoff cannot start from %s", core.ErrConflict, item.State)
	}
	handoff, err := s.handoffs.Create(ctx, id)
	if err != nil {
		return WebHandoffOutput{}, err
	}
	return WebHandoffOutput{ConsultationID: id, ReadToken: handoff.ReadToken, ExpiresAt: handoff.ExpiresAt}, nil
}

func (s *ConsultationService) ImportWebResult(id, payload string) (consultation.Consultation, error) {
	ctx := context.Background()
	if err := s.validate(); err != nil {
		return consultation.Consultation{}, err
	}
	var result resultcontract.Result
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		return consultation.Consultation{}, fmt.Errorf("%w: invalid result JSON: %v", core.ErrInvalidArgument, err)
	}
	updated, _, err := s.app.StoreResultAttested(ctx, id, "user_declared:chatgpt_web_handoff", result)
	return updated, err
}

func (s *ConsultationService) prepareRun(ctx context.Context, id string) (consultation.Consultation, error) {
	item, err := s.app.Consultations.Get(ctx, id)
	if err != nil {
		return consultation.Consultation{}, err
	}
	if item.State == consultation.StateApprovalPending {
		item, err = s.app.Consultations.Approve(ctx, id)
		if err != nil {
			return consultation.Consultation{}, err
		}
	}
	if item.State == consultation.StateQueued {
		item, err = s.app.Consultations.Advance(ctx, id, consultation.EventStarted)
		if err != nil {
			return consultation.Consultation{}, err
		}
	}
	if item.State != consultation.StateRunning {
		return consultation.Consultation{}, fmt.Errorf("%w: consultation is %s", core.ErrConflict, item.State)
	}
	return item, nil
}

func (s *ConsultationService) validate() error {
	if s == nil || s.app == nil || s.app.Consultations == nil {
		return fmt.Errorf("%w: consultation application", core.ErrInvalidConfiguration)
	}
	return nil
}

func expertPricingFromEnvironment() (*openaiapi.Pricing, int64, error) {
	inputRaw := strings.TrimSpace(os.Getenv("EXPERT_INPUT_PER_MILLION_MINOR"))
	outputRaw := strings.TrimSpace(os.Getenv("EXPERT_OUTPUT_PER_MILLION_MINOR"))
	reasoningRaw := strings.TrimSpace(os.Getenv("EXPERT_REASONING_PER_MILLION_MINOR"))
	maximumOutputRaw := strings.TrimSpace(os.Getenv("EXPERT_MAX_OUTPUT_TOKENS"))

	maximumOutputTokens, err := parseOptionalNonNegativeInt64("EXPERT_MAX_OUTPUT_TOKENS", maximumOutputRaw)
	if err != nil {
		return nil, 0, err
	}
	if inputRaw == "" && outputRaw == "" && reasoningRaw == "" {
		return nil, maximumOutputTokens, nil
	}
	if inputRaw == "" || outputRaw == "" {
		return nil, 0, fmt.Errorf("%w: EXPERT_INPUT_PER_MILLION_MINOR and EXPERT_OUTPUT_PER_MILLION_MINOR are both required", core.ErrInvalidConfiguration)
	}
	input, err := parseOptionalNonNegativeInt64("EXPERT_INPUT_PER_MILLION_MINOR", inputRaw)
	if err != nil {
		return nil, 0, err
	}
	output, err := parseOptionalNonNegativeInt64("EXPERT_OUTPUT_PER_MILLION_MINOR", outputRaw)
	if err != nil {
		return nil, 0, err
	}
	reasoning := output
	if reasoningRaw != "" {
		reasoning, err = parseOptionalNonNegativeInt64("EXPERT_REASONING_PER_MILLION_MINOR", reasoningRaw)
		if err != nil {
			return nil, 0, err
		}
	}
	return &openaiapi.Pricing{
		InputPerMillionMinor:     input,
		OutputPerMillionMinor:    output,
		ReasoningPerMillionMinor: reasoning,
	}, maximumOutputTokens, nil
}

func parseOptionalNonNegativeInt64(name, raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%w: %s must be a non-negative integer", core.ErrInvalidConfiguration, name)
	}
	return value, nil
}
