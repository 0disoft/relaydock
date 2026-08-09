package controlaccess

import "testing"

const (
	tenantA  = "00000000-0000-0000-0000-000000000001"
	projectA = "00000000-0000-0000-0000-000000000002"
)

func TestAuthorizeUsesDenyByDefaultRoleMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		principal Principal
		action    Action
		allowed   bool
	}{
		{"gateway reads snapshot", Principal{Subject: "gateway-1", Role: RoleGateway}, ActionSnapshotRead, true},
		{"gateway cannot publish", Principal{Subject: "gateway-1", Role: RoleGateway}, ActionSnapshotPublish, false},
		{"viewer reads models", Principal{Subject: "viewer-1", Role: RoleViewer, TenantID: tenantA, ProjectID: projectA}, ActionModelsRead, true},
		{"viewer cannot read snapshot", Principal{Subject: "viewer-1", Role: RoleViewer, TenantID: tenantA, ProjectID: projectA}, ActionSnapshotRead, false},
		{"publisher publishes", Principal{Subject: "publisher-1", Role: RolePublisher}, ActionSnapshotPublish, true},
		{"admin uses known action", Principal{Subject: "admin-1", Role: RoleAdmin}, ActionSigningKeyRead, true},
		{"admin denied unknown action", Principal{Subject: "admin-1", Role: RoleAdmin}, Action("control:future"), false},
		{"unknown role denied", Principal{Subject: "unknown-1", Role: Role("future")}, ActionModelsRead, false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			decision := Authorize(test.principal, test.action)
			if decision.Allowed != test.allowed {
				t.Fatalf("Authorize(%#v, %q) allowed=%v, want %v (%s)", test.principal, test.action, decision.Allowed, test.allowed, decision.Reason)
			}
			if decision.PolicyVersion != PolicyVersion {
				t.Fatalf("policy version %q, want %q", decision.PolicyVersion, PolicyVersion)
			}
		})
	}
}

func TestPrincipalRejectsScopedPrivilegeRoles(t *testing.T) {
	t.Parallel()
	for _, role := range []Role{RoleGateway, RolePublisher, RoleAdmin} {
		principal := Principal{Subject: "principal", Role: role, TenantID: tenantA, ProjectID: projectA}
		if err := principal.Validate(); err == nil {
			t.Fatalf("role %q accepted project scope", role)
		}
	}
}
