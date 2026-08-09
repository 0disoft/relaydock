package mcpscope

import (
	"errors"
	"testing"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

func TestValidateIssuedClaimsRequiresTenantProjectForNarrowScopes(t *testing.T) {
	for _, scopes := range [][]string{{ConsultationsRead}, {ConsultationsAnswer}, {ConsultationsRead, ConsultationsAnswer}} {
		if err := ValidateIssuedClaims(scopes, "tenant-a", "project-a"); err != nil {
			t.Fatalf("valid scoped token rejected: %v", err)
		}
		if err := ValidateIssuedClaims(scopes, "", ""); !errors.Is(err, core.ErrInvalidArgument) {
			t.Fatalf("unscoped token accepted: %v", err)
		}
	}
}

func TestValidateIssuedClaimsAllowsOnlyStandaloneAdmin(t *testing.T) {
	if err := ValidateIssuedClaims([]string{ConsultationsAdmin}, "", ""); err != nil {
		t.Fatalf("cluster admin rejected: %v", err)
	}
	if err := ValidateIssuedClaims([]string{ConsultationsAdmin}, "tenant", "project"); !errors.Is(err, core.ErrInvalidArgument) {
		t.Fatalf("tenant-labelled cluster admin accepted: %v", err)
	}
	if err := ValidateIssuedClaims([]string{ConsultationsAdmin, ConsultationsRead}, "", ""); !errors.Is(err, core.ErrInvalidArgument) {
		t.Fatalf("mixed admin token accepted: %v", err)
	}
}

func TestNormalizeRejectsUnknownAndWildcardScopes(t *testing.T) {
	for _, scope := range []string{"*", "consultations:write", "models:invoke"} {
		if _, err := Normalize([]string{scope}); !errors.Is(err, core.ErrInvalidArgument) {
			t.Fatalf("scope %q accepted: %v", scope, err)
		}
	}
}

func TestAllowsAdminToUseNarrowerTools(t *testing.T) {
	if !Allows([]string{ConsultationsAdmin}, ConsultationsRead) || !Allows([]string{ConsultationsAdmin}, ConsultationsAnswer) {
		t.Fatal("admin scope did not satisfy consultation tool permissions")
	}
	if Allows([]string{ConsultationsRead}, ConsultationsAnswer) {
		t.Fatal("read scope satisfied answer permission")
	}
}
