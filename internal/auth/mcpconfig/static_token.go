package mcpconfig

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

// StaticToken resolves the legacy Remote MCP bearer credential without
// silently widening the authority of the broker administration credential.
// When scoped tokens are configured, broker-token reuse is disabled unless an
// operator explicitly opts in. Without scoped tokens, the historical local
// fallback remains enabled for compatibility.
func StaticToken(explicitToken, brokerToken, rawAllowBroker string, scopedTokensConfigured bool) (string, error) {
	explicitToken = strings.TrimSpace(explicitToken)
	if explicitToken != "" {
		return explicitToken, nil
	}
	allowBroker, err := parseOptionalBoolean(
		"EXPERT_MCP_ALLOW_BROKER_TOKEN",
		rawAllowBroker,
		!scopedTokensConfigured,
	)
	if err != nil {
		return "", err
	}
	if !allowBroker {
		return "", nil
	}
	return strings.TrimSpace(brokerToken), nil
}

func parseOptionalBoolean(name, raw string, fallback bool) (bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%w: %s must be a boolean", core.ErrInvalidConfiguration, name)
	}
	return value, nil
}
