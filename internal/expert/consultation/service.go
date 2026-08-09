package consultation

import (
	"context"
	"fmt"
	"strings"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (Consultation, error) {
	if s == nil || s.repository == nil {
		return Consultation{}, fmt.Errorf("%w: consultation repository", core.ErrInvalidConfiguration)
	}
	cmd.Objective = strings.TrimSpace(cmd.Objective)
	if cmd.Objective == "" {
		return Consultation{}, fmt.Errorf("%w: objective is required", core.ErrInvalidArgument)
	}
	if cmd.Route == "" {
		cmd.Route = RouteOpenAIAPIPro
	}
	if cmd.Route != RouteOpenAIAPIPro && cmd.Route != RouteWebHandoff {
		return Consultation{}, fmt.Errorf("%w: route %q", core.ErrInvalidArgument, cmd.Route)
	}
	if cmd.MaximumCostMinor < 0 {
		return Consultation{}, fmt.Errorf("%w: negative cost limit", core.ErrInvalidArgument)
	}
	return s.repository.Create(ctx, cmd)
}
func (s *Service) Get(ctx context.Context, id string) (Consultation, error) {
	return s.repository.Get(ctx, id)
}
func (s *Service) List(ctx context.Context, limit int) ([]Consultation, error) {
	return s.repository.List(ctx, limit)
}
func (s *Service) Advance(ctx context.Context, id string, event Event) (Consultation, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return Consultation{}, err
	}
	next, err := Transition(current.State, event)
	if err != nil {
		return Consultation{}, err
	}
	return s.repository.UpdateState(ctx, id, current.State, next)
}
func (s *Service) Cancel(ctx context.Context, id string) (Consultation, error) {
	return s.Advance(ctx, id, EventCancelled)
}
func (s *Service) Approve(ctx context.Context, id string) (Consultation, error) {
	return s.Advance(ctx, id, EventApproved)
}
func (s *Service) AttachContext(ctx context.Context, id, packID string) (Consultation, error) {
	if strings.TrimSpace(packID) == "" {
		return Consultation{}, core.ErrInvalidArgument
	}
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return Consultation{}, err
	}
	next, err := Transition(current.State, EventContextAttached)
	if err != nil {
		return Consultation{}, err
	}
	return s.repository.AttachContext(ctx, id, packID, current.State, next)
}
func (s *Service) SubmitResult(ctx context.Context, id, resultID string) (Consultation, error) {
	c, err := s.repository.AttachResult(ctx, id, resultID)
	if err != nil {
		return Consultation{}, err
	}
	return s.repository.UpdateState(ctx, id, c.State, StateCompleted)
}
