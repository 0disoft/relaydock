package virtualkey

import (
	"context"
	"strings"

	"github.com/0disoft/relaydock/internal/auth/authorization"
	"github.com/0disoft/relaydock/internal/core"
)

type Principal struct {
	TenantID      string
	ProjectID     string
	VirtualKeyID  string
	Scopes        []string
	AllowedModels []string
}

type Authenticator interface {
	Authenticate(context.Context, string) (Principal, error)
}

func (p Principal) Authorize(scope, model string) error {
	if !authorization.New(p.Scopes...).Has(scope) {
		return core.ErrForbidden
	}
	if strings.TrimSpace(model) == "" || len(p.AllowedModels) == 0 {
		return nil
	}
	for _, allowed := range p.AllowedModels {
		if strings.EqualFold(strings.TrimSpace(allowed), strings.TrimSpace(model)) {
			return nil
		}
	}
	return core.ErrForbidden
}
