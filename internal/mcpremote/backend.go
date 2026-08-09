package mcpremote

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/0disoft/relaydock/internal/auth/scopedtoken"
	"github.com/0disoft/relaydock/internal/core"
	expertapp "github.com/0disoft/relaydock/internal/expert/app"
	"github.com/0disoft/relaydock/internal/expert/consultation"
	"github.com/0disoft/relaydock/internal/expert/contextpack"
	"github.com/0disoft/relaydock/internal/expert/resultcontract"
)

type ServiceBackend struct {
	App *expertapp.App
}

func NewServiceBackend(app *expertapp.App) *ServiceBackend {
	return &ServiceBackend{App: app}
}

func (b *ServiceBackend) GetConsultation(ctx context.Context, input ConsultationGetInput) (ConsultationGetOutput, error) {
	if b == nil || b.App == nil {
		return ConsultationGetOutput{}, fmt.Errorf("%w: expert application", core.ErrInvalidConfiguration)
	}
	id := strings.TrimSpace(input.ConsultationID)
	if id == "" {
		return ConsultationGetOutput{}, fmt.Errorf("%w: consultationId", core.ErrInvalidArgument)
	}
	item, err := b.App.Consultations.Get(ctx, id)
	if err != nil {
		return ConsultationGetOutput{}, err
	}
	if err := authorizeConsultationScope(ctx, item); err != nil {
		return ConsultationGetOutput{}, err
	}
	if item.State != consultation.StateQueued && item.State != consultation.StateRunning && item.State != consultation.StateResultPending && item.State != consultation.StateCompleted {
		return ConsultationGetOutput{}, fmt.Errorf("%w: consultation is %s", core.ErrConflict, item.State)
	}
	pack, err := b.App.Packs.Get(ctx, item.ContextPackID)
	if err != nil {
		return ConsultationGetOutput{}, err
	}
	if err := validatePackScope(item, pack); err != nil {
		return ConsultationGetOutput{}, err
	}
	return ConsultationGetOutput{
		ConsultationID: item.ID,
		Objective:      item.Objective,
		TaskType:       item.TaskType,
		ContextPack:    pack,
		State:          string(item.State),
		MaximumCost:    item.MaximumCostMinor,
	}, nil
}

func (b *ServiceBackend) SubmitResult(ctx context.Context, input SubmitResultInput) (SubmitResultOutput, error) {
	if b == nil || b.App == nil {
		return SubmitResultOutput{}, fmt.Errorf("%w: expert application", core.ErrInvalidConfiguration)
	}
	id := strings.TrimSpace(input.ConsultationID)
	if id == "" {
		return SubmitResultOutput{}, fmt.Errorf("%w: consultationId", core.ErrInvalidArgument)
	}
	item, err := b.App.Consultations.Get(ctx, id)
	if err != nil {
		return SubmitResultOutput{}, err
	}
	if err := authorizeConsultationScope(ctx, item); err != nil {
		return SubmitResultOutput{}, err
	}
	attestation := strings.TrimSpace(input.ModelAttestation)
	if attestation == "" {
		return SubmitResultOutput{}, fmt.Errorf("%w: modelAttestation", core.ErrInvalidArgument)
	}
	payload, err := json.Marshal(input.StructuredResult)
	if err != nil {
		return SubmitResultOutput{}, fmt.Errorf("marshal structured result: %w", err)
	}
	var result resultcontract.Result
	if err := json.Unmarshal(payload, &result); err != nil {
		return SubmitResultOutput{}, fmt.Errorf("%w: malformed structured result: %v", core.ErrInvalidArgument, err)
	}
	updated, resultID, err := b.App.StoreResultAttested(ctx, id, attestation, result)
	if err != nil {
		return SubmitResultOutput{}, err
	}
	return SubmitResultOutput{
		ConsultationID:   updated.ID,
		ResultID:         resultID,
		State:            string(updated.State),
		ModelAttestation: attestation,
	}, nil
}

func authorizeConsultationScope(ctx context.Context, item consultation.Consultation) error {
	claims, ok := scopedtoken.FromContext(ctx)
	if !ok {
		return core.ErrUnauthorized
	}
	// Static compatibility and explicitly administrative credentials are
	// trusted cluster-level principals. Newly issued scoped credentials always
	// carry a token ID and must match both tenant and project exactly.
	if claims.TokenID == "" || claims.HasScope(ScopeConsultationsAdmin) {
		return nil
	}
	if strings.TrimSpace(claims.TenantID) != strings.TrimSpace(item.TenantID) || strings.TrimSpace(claims.ProjectID) != strings.TrimSpace(item.ProjectID) {
		return core.ErrForbidden
	}
	return nil
}

func validatePackScope(item consultation.Consultation, pack contextpack.Pack) error {
	if strings.TrimSpace(item.ContextPackID) == "" || pack.ID != item.ContextPackID {
		return fmt.Errorf("%w: consultation context pack reference", core.ErrCorruptState)
	}
	if strings.TrimSpace(pack.TenantID) != strings.TrimSpace(item.TenantID) || strings.TrimSpace(pack.ProjectID) != strings.TrimSpace(item.ProjectID) {
		return fmt.Errorf("%w: consultation and ContextPack scope disagree", core.ErrCorruptState)
	}
	return nil
}
