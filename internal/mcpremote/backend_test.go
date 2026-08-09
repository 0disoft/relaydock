package mcpremote

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/auth/scopedtoken"
	"github.com/your-org/ai-runtime-gateway/internal/core"
	expertapp "github.com/your-org/ai-runtime-gateway/internal/expert/app"
	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
)

func TestServiceBackendRejectsCrossProjectRead(t *testing.T) {
	backend, item := newQueuedConsultation(t, "tenant-a", "project-a")
	ctx := scopedContext("tenant-a", "project-b", ScopeConsultationsRead)

	_, err := backend.GetConsultation(ctx, ConsultationGetInput{ConsultationID: item.ID})
	if !errors.Is(err, core.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestServiceBackendAllowsMatchingScopedRead(t *testing.T) {
	backend, item := newQueuedConsultation(t, "tenant-a", "project-a")
	ctx := scopedContext("tenant-a", "project-a", ScopeConsultationsRead)

	output, err := backend.GetConsultation(ctx, ConsultationGetInput{ConsultationID: item.ID})
	if err != nil {
		t.Fatalf("get consultation: %v", err)
	}
	if output.ConsultationID != item.ID || output.ContextPack.TenantID != "tenant-a" || output.ContextPack.ProjectID != "project-a" {
		t.Fatalf("unexpected output: %#v", output)
	}
}

func TestServiceBackendAdminScopeCanReadAcrossProjects(t *testing.T) {
	backend, item := newQueuedConsultation(t, "tenant-a", "project-a")
	ctx := scopedContext("tenant-b", "project-b", ScopeConsultationsAdmin)

	if _, err := backend.GetConsultation(ctx, ConsultationGetInput{ConsultationID: item.ID}); err != nil {
		t.Fatalf("admin read: %v", err)
	}
}

func TestServiceBackendRejectsContextPackScopeMismatch(t *testing.T) {
	backend, item := newQueuedConsultation(t, "tenant-a", "project-a")
	pack, err := backend.App.Packs.Get(context.Background(), item.ContextPackID)
	if err != nil {
		t.Fatalf("get pack: %v", err)
	}
	pack.ProjectID = "project-b"
	if err := backend.App.Packs.Put(context.Background(), pack); err != nil {
		t.Fatalf("overwrite pack: %v", err)
	}

	_, err = backend.GetConsultation(scopedContext("tenant-a", "project-a", ScopeConsultationsRead), ConsultationGetInput{ConsultationID: item.ID})
	if !errors.Is(err, core.ErrCorruptState) {
		t.Fatalf("expected corrupt state, got %v", err)
	}
}

func TestServiceBackendRejectsCrossTenantResultBeforeMutation(t *testing.T) {
	backend, item := newQueuedConsultation(t, "tenant-a", "project-a")
	ctx := scopedContext("tenant-b", "project-a", ScopeConsultationsAnswer)

	_, err := backend.SubmitResult(ctx, SubmitResultInput{
		ConsultationID:   item.ID,
		ModelAttestation: "user_declared",
		StructuredResult: map[string]any{"decision": "must not be stored", "confidence": 1},
	})
	if !errors.Is(err, core.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	current, getErr := backend.App.Consultations.Get(context.Background(), item.ID)
	if getErr != nil {
		t.Fatalf("get consultation: %v", getErr)
	}
	if current.State != consultation.StateQueued || current.ResultID != "" {
		t.Fatalf("unauthorized write mutated consultation: %#v", current)
	}
}

func TestServiceBackendRequiresAuthenticatedClaims(t *testing.T) {
	backend, item := newQueuedConsultation(t, "tenant-a", "project-a")
	_, err := backend.GetConsultation(context.Background(), ConsultationGetInput{ConsultationID: item.ID})
	if !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func newQueuedConsultation(t *testing.T, tenantID, projectID string) (*ServiceBackend, consultation.Consultation) {
	t.Helper()
	app, err := expertapp.NewInMemory()
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	pack := contextpack.Pack{
		ID:        "ctx-scope-test",
		TenantID:  tenantID,
		ProjectID: projectID,
		Objective: "review tenancy boundaries",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := app.Packs.Put(context.Background(), pack); err != nil {
		t.Fatalf("put pack: %v", err)
	}
	item, err := app.Consultations.Create(context.Background(), consultation.CreateCommand{
		TenantID:      tenantID,
		ProjectID:     projectID,
		Objective:     pack.Objective,
		ContextPackID: pack.ID,
		Route:         consultation.RouteWebHandoff,
		TTL:           time.Hour,
	})
	if err != nil {
		t.Fatalf("create consultation: %v", err)
	}
	item, err = app.Consultations.Approve(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("approve consultation: %v", err)
	}
	return NewServiceBackend(app), item
}

func scopedContext(tenantID, projectID string, scopes ...string) context.Context {
	return scopedtoken.WithClaims(context.Background(), scopedtoken.Claims{
		Subject:   "test-principal",
		TenantID:  tenantID,
		ProjectID: projectID,
		Scopes:    scopes,
		TokenID:   "token-test",
	})
}

func TestRequireRemoteScopeAllowsClusterAdmin(t *testing.T) {
	ctx := scopedtoken.WithClaims(context.Background(), scopedtoken.Claims{Scopes: []string{ScopeConsultationsAdmin}})
	if err := requireRemoteScope(ctx, ScopeConsultationsRead); err != nil {
		t.Fatalf("admin could not read consultation: %v", err)
	}
	if err := requireRemoteScope(ctx, ScopeConsultationsAnswer); err != nil {
		t.Fatalf("admin could not answer consultation: %v", err)
	}
}

func TestRequireRemoteScopeDoesNotPromoteReadToAnswer(t *testing.T) {
	ctx := scopedtoken.WithClaims(context.Background(), scopedtoken.Claims{Scopes: []string{ScopeConsultationsRead}})
	if err := requireRemoteScope(ctx, ScopeConsultationsAnswer); !errors.Is(err, core.ErrForbidden) {
		t.Fatalf("read scope answered consultation: %v", err)
	}
}
