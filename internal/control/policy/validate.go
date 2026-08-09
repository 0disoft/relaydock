package policy

import (
	"fmt"
	"strings"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

const (
	LossModeStrict      = "strict"
	LossModeCompatible  = "compatible"
	LossModePassthrough = "passthrough"
)

func (p *RoutingPolicy) Validate() error {
	if p == nil {
		return fmt.Errorf("%w: routing policy", core.ErrInvalidArgument)
	}
	p.ID = strings.TrimSpace(p.ID)
	if p.ID == "" {
		return fmt.Errorf("%w: routing policy id", core.ErrInvalidArgument)
	}
	if p.MaximumRequestCost < 0 {
		return fmt.Errorf("%w: negative maximum request cost", core.ErrInvalidArgument)
	}
	if p.RetryBeforeStream < 0 || p.RetryBeforeStream > 3 {
		return fmt.Errorf("%w: retry-before-stream must be between 0 and 3", core.ErrInvalidArgument)
	}
	if p.LossMode == "" {
		p.LossMode = LossModeStrict
	}
	switch p.LossMode {
	case LossModeStrict, LossModeCompatible, LossModePassthrough:
	default:
		return fmt.Errorf("%w: unknown loss mode %q", core.ErrInvalidArgument, p.LossMode)
	}
	if p.PayloadRetention == "" {
		p.PayloadRetention = "none"
	}
	switch p.PayloadRetention {
	case "none", "metadata", "redacted", "full":
	default:
		return fmt.Errorf("%w: unknown payload retention %q", core.ErrInvalidArgument, p.PayloadRetention)
	}
	p.AllowedProviders = normalizedUnique(p.AllowedProviders)
	p.AllowedRegions = normalizedUnique(p.AllowedRegions)
	return nil
}

func (p RoutingPolicy) AllowsProvider(provider string) bool {
	return allows(p.AllowedProviders, provider)
}

func (p RoutingPolicy) AllowsRegion(region string) bool {
	return allows(p.AllowedRegions, region)
}

func normalizedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func allows(allowed []string, value string) bool {
	if len(allowed) == 0 {
		return true
	}
	value = strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range allowed {
		if candidate == value {
			return true
		}
	}
	return false
}
