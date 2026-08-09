// Package controlaccess owns authentication and authorization contracts for
// the RelayDock Control Plane. Authentication establishes a principal;
// authorization remains a separate deny-by-default decision.
package controlaccess

import (
	"context"
	"fmt"
	"strings"

	"github.com/0disoft/relaydock/internal/identifier"
)

const PolicyVersion = "control-access/v1"

type Role string

const (
	RoleGateway   Role = "gateway"
	RoleViewer    Role = "viewer"
	RolePublisher Role = "publisher"
	RoleAdmin     Role = "admin"
)

type Action string

const (
	ActionAuthenticate    Action = "control:authenticate"
	ActionSnapshotRead    Action = "control:snapshot:read"
	ActionSnapshotWatch   Action = "control:snapshot:watch"
	ActionSnapshotPublish Action = "control:snapshot:publish"
	ActionSigningKeyRead  Action = "control:signing-key:read"
	ActionModelsRead      Action = "control:models:read"
)

type Principal struct {
	Subject   string `json:"subject"`
	Role      Role   `json:"role"`
	TenantID  string `json:"tenantId,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
}

func (p *Principal) Validate() error {
	if p == nil {
		return fmt.Errorf("principal is required")
	}
	p.Subject = strings.TrimSpace(p.Subject)
	p.TenantID = strings.TrimSpace(p.TenantID)
	p.ProjectID = strings.TrimSpace(p.ProjectID)
	if p.Subject == "" || len(p.Subject) > 128 {
		return fmt.Errorf("subject must contain between 1 and 128 characters")
	}
	for _, value := range p.Subject {
		if value < 0x21 || value > 0x7e {
			return fmt.Errorf("subject must contain printable ASCII without whitespace")
		}
	}
	switch p.Role {
	case RoleGateway, RoleViewer, RolePublisher, RoleAdmin:
	default:
		return fmt.Errorf("unknown control role %q", p.Role)
	}
	if p.TenantID != "" && !identifier.IsUUID(p.TenantID) {
		return fmt.Errorf("tenantId must be a canonical UUID")
	}
	if p.ProjectID != "" && !identifier.IsUUID(p.ProjectID) {
		return fmt.Errorf("projectId must be a canonical UUID")
	}
	if p.ProjectID != "" && p.TenantID == "" {
		return fmt.Errorf("projectId requires tenantId")
	}
	if p.Role != RoleViewer && (p.TenantID != "" || p.ProjectID != "") {
		return fmt.Errorf("role %q requires cluster scope", p.Role)
	}
	return nil
}

func (p Principal) IsClusterScoped() bool {
	return strings.TrimSpace(p.TenantID) == "" && strings.TrimSpace(p.ProjectID) == ""
}

type Decision struct {
	Allowed       bool
	Reason        string
	PolicyVersion string
}

type AuditEvent struct {
	Subject       string
	Role          Role
	TenantID      string
	ProjectID     string
	Action        Action
	Allowed       bool
	Reason        string
	PolicyVersion string
}

type AuditFunc func(context.Context, AuditEvent)

func Authorize(principal Principal, action Action) Decision {
	decision := Decision{Reason: "permission_denied", PolicyVersion: PolicyVersion}
	if err := principal.Validate(); err != nil {
		decision.Reason = "invalid_principal"
		return decision
	}

	switch principal.Role {
	case RoleAdmin:
		decision.Allowed = principal.IsClusterScoped() && knownAction(action)
	case RolePublisher:
		decision.Allowed = principal.IsClusterScoped() && knownAction(action)
	case RoleGateway:
		decision.Allowed = principal.IsClusterScoped() && (action == ActionSnapshotRead || action == ActionSnapshotWatch || action == ActionSigningKeyRead || action == ActionModelsRead)
	case RoleViewer:
		decision.Allowed = action == ActionModelsRead
	}
	if decision.Allowed {
		decision.Reason = "allowed"
	}
	return decision
}

func knownAction(action Action) bool {
	switch action {
	case ActionSnapshotRead, ActionSnapshotWatch, ActionSnapshotPublish, ActionSigningKeyRead, ActionModelsRead:
		return true
	default:
		return false
	}
}

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}
