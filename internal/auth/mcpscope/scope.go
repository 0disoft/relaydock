package mcpscope

import (
	"fmt"
	"sort"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
)

const (
	ConsultationsRead   = "consultations:read"
	ConsultationsAnswer = "consultations:answer"
	ConsultationsAdmin  = "consultations:admin"
)

var allowed = map[string]struct{}{
	ConsultationsRead:   {},
	ConsultationsAnswer: {},
	ConsultationsAdmin:  {},
}

// ValidateIssuedClaims constrains operator-issued Remote MCP credentials to the
// public consultation permission surface. Read and answer credentials are
// always tenant/project scoped. Cluster-wide access requires the explicit
// admin scope, preventing an accidentally unscoped read token from becoming a
// cross-tenant credential.
func ValidateIssuedClaims(scopes []string, tenantID, projectID string) error {
	normalized, err := Normalize(scopes)
	if err != nil {
		return err
	}
	tenantID = strings.TrimSpace(tenantID)
	projectID = strings.TrimSpace(projectID)
	if (tenantID == "") != (projectID == "") {
		return fmt.Errorf("%w: tenant and project must be supplied together", core.ErrInvalidArgument)
	}
	admin := contains(normalized, ConsultationsAdmin)
	if admin {
		if len(normalized) != 1 {
			return fmt.Errorf("%w: consultations:admin must not be combined with narrower scopes", core.ErrInvalidArgument)
		}
		if tenantID != "" || projectID != "" {
			return fmt.Errorf("%w: consultations:admin is cluster-wide and must not carry tenant or project", core.ErrInvalidArgument)
		}
		return nil
	}
	if tenantID == "" || projectID == "" {
		return fmt.Errorf("%w: consultation read/answer tokens require tenant and project", core.ErrInvalidArgument)
	}
	return nil
}

func Normalize(scopes []string) ([]string, error) {
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := allowed[scope]; !ok {
			return nil, fmt.Errorf("%w: unsupported Remote MCP scope %q", core.ErrInvalidArgument, scope)
		}
		seen[scope] = struct{}{}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("%w: at least one Remote MCP scope is required", core.ErrInvalidArgument)
	}
	out := make([]string, 0, len(seen))
	for scope := range seen {
		out = append(out, scope)
	}
	sort.Strings(out)
	return out, nil
}

// Allows applies the Remote MCP scope hierarchy. The explicit admin scope is
// cluster-wide and therefore satisfies the narrower tool permissions.
func Allows(scopes []string, required string) bool {
	required = strings.TrimSpace(required)
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == required || scope == ConsultationsAdmin {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
