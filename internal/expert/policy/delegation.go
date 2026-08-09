package policy

import (
	"fmt"

	"github.com/0disoft/relaydock/internal/core"
)

type DelegationGuard struct {
	MaximumDepth int
}

func NewDelegationGuard(maximumDepth int) DelegationGuard {
	if maximumDepth < 0 {
		maximumDepth = 0
	}
	return DelegationGuard{MaximumDepth: maximumDepth}
}

func (g DelegationGuard) Check(depth int) error {
	if depth < 0 {
		return fmt.Errorf("%w: delegation depth cannot be negative", core.ErrInvalidArgument)
	}
	if depth > g.MaximumDepth {
		return fmt.Errorf("%w: delegation depth %d exceeds maximum %d", core.ErrForbidden, depth, g.MaximumDepth)
	}
	return nil
}
