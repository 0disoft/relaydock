package scopedtoken

import (
	"fmt"
	"strings"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

func validateClaimsShape(claims Claims) error {
	if claims.Subject != strings.TrimSpace(claims.Subject) || claims.Subject == "" || len(claims.Subject) > maximumSubjectBytes {
		return fmt.Errorf("%w: scoped token subject", core.ErrInvalidArgument)
	}
	if claims.TenantID != strings.TrimSpace(claims.TenantID) || claims.ProjectID != strings.TrimSpace(claims.ProjectID) || claims.Audience != strings.TrimSpace(claims.Audience) || len(claims.TenantID) > maximumIdentityBytes || len(claims.ProjectID) > maximumIdentityBytes || len(claims.Audience) > maximumIdentityBytes {
		return fmt.Errorf("%w: scoped token identity field", core.ErrInvalidArgument)
	}
	if claims.TokenID != strings.TrimSpace(claims.TokenID) || claims.TokenID == "" || len(claims.TokenID) > maximumIdentityBytes {
		return fmt.Errorf("%w: scoped token ID", core.ErrInvalidArgument)
	}
	if len(claims.Scopes) == 0 || len(claims.Scopes) > maximumScopeCount {
		return fmt.Errorf("%w: scoped token scopes", core.ErrInvalidArgument)
	}
	for _, scope := range claims.Scopes {
		if scope != strings.TrimSpace(scope) || scope == "" || len(scope) > maximumScopeBytes {
			return fmt.Errorf("%w: scoped token scope", core.ErrInvalidArgument)
		}
	}
	return nil
}
