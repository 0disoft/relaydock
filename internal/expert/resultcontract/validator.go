package resultcontract

import (
	"fmt"
	"github.com/your-org/ai-runtime-gateway/internal/core"
	"strings"
)

func Validate(r Result) error {
	if strings.TrimSpace(r.Decision) == "" {
		return fmt.Errorf("%w: decision is required", core.ErrInvalidArgument)
	}
	if r.Confidence < 0 || r.Confidence > 1 {
		return fmt.Errorf("%w: confidence must be between 0 and 1", core.ErrInvalidArgument)
	}
	for i, f := range r.CriticalFindings {
		if strings.TrimSpace(f.Summary) == "" {
			return fmt.Errorf("%w: finding %d has no summary", core.ErrInvalidArgument, i)
		}
		switch f.Severity {
		case "critical", "high", "medium", "low", "info", "":
		default:
			return fmt.Errorf("%w: finding %d severity %q", core.ErrInvalidArgument, i, f.Severity)
		}
	}
	return nil
}
