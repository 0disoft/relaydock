package gateway

/* llmnav/1 module
id=relaydock.routing.candidates
role=Build and serve validated provider candidates for virtual and direct model routes under runtime availability state.
owns=virtual model candidate catalog|direct routing fallback|candidate runtime state projection
excludes=route scoring|provider request execution
search=resolve virtual model route|provider candidate source|direct model routing
invariant=Configured routes reference enabled providers and supported protocols.
invariant=Expired signed runtime state prevents candidate selection.
stability=architecture
*/

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/routing"
)

func newCandidateSource(definitions map[string]providerDefinition, routes map[string][]RouteConfig, defaultProvider string) (*CandidateSource, error) {
	source := &CandidateSource{
		routes:               make(map[string][]routing.Candidate, len(routes)+1),
		providers:            definitions,
		defaultProvider:      strings.TrimSpace(defaultProvider),
		states:               make(map[string]candidateState),
		now:                  func() time.Time { return time.Now().UTC() },
		allowDirectRouting:   true,
		maximumRuntimeStates: defaultMaximumRuntimeStates,
		runtimeStateTTL:      defaultRuntimeStateTTL,
	}
	if mock, exists := definitions["mock"]; exists && mock.Configured {
		source.routes["local/echo"] = []routing.Candidate{candidateFromDefinition("mock", "local", "local/echo", canonical.ProtocolOpenAIResponses, mock, RouteConfig{HealthScore: 1, ConcurrencyLimit: 64})}
	}
	for virtualModel, configuredRoutes := range routes {
		virtualModel = strings.TrimSpace(virtualModel)
		if virtualModel == "" {
			return nil, fmt.Errorf("%w: empty virtual model route", core.ErrInvalidConfiguration)
		}
		if len(configuredRoutes) == 0 {
			return nil, fmt.Errorf("%w: route %s has no candidates", core.ErrInvalidConfiguration, virtualModel)
		}
		candidates := make([]routing.Candidate, 0, len(configuredRoutes))
		for _, configured := range configuredRoutes {
			if configured.Disabled {
				continue
			}
			providerName := strings.TrimSpace(configured.Provider)
			definition, exists := definitions[providerName]
			if !exists {
				return nil, fmt.Errorf("%w: route %s references provider %s", core.ErrInvalidConfiguration, virtualModel, providerName)
			}
			if !definition.Configured {
				return nil, fmt.Errorf("%w: route %s references unconfigured provider %s", core.ErrInvalidConfiguration, virtualModel, providerName)
			}
			if strings.TrimSpace(configured.Model) == "" {
				return nil, fmt.Errorf("%w: route %s has an empty upstream model", core.ErrInvalidConfiguration, virtualModel)
			}
			protocol := configured.Protocol
			if protocol == "" {
				protocol = definition.Protocol
			}
			if err := validateProtocol(protocol); err != nil {
				return nil, err
			}
			accountID := strings.TrimSpace(configured.AccountID)
			if accountID == "" {
				accountID = providerName + "/default"
			}
			candidates = append(candidates, candidateFromDefinition(providerName, accountID, configured.Model, protocol, definition, configured))
		}
		if len(candidates) == 0 {
			return nil, fmt.Errorf("%w: route %s has no enabled candidates", core.ErrInvalidConfiguration, virtualModel)
		}
		source.routes[virtualModel] = candidates
	}
	if source.defaultProvider != "" {
		definition, exists := definitions[source.defaultProvider]
		if !exists || !definition.Configured {
			return nil, fmt.Errorf("%w: default provider %s is unavailable", core.ErrInvalidConfiguration, source.defaultProvider)
		}
	}
	return source, nil
}

func (s *CandidateSource) Candidates(ctx context.Context, requestedModel string) ([]routing.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return nil, fmt.Errorf("%w: model is required", core.ErrInvalidArgument)
	}
	if !s.Ready() {
		return nil, fmt.Errorf("%w: signed runtime snapshot is unavailable or expired", core.ErrExpired)
	}
	s.routeMu.RLock()
	candidates, exists := s.routes[requestedModel]
	if exists {
		candidates = cloneCandidates(candidates)
	}
	s.routeMu.RUnlock()
	if exists {
		return s.applyRuntimeState(candidates), nil
	}
	if !s.DirectRoutingAllowed() {
		return nil, core.ErrNoRoute
	}
	providerName, upstreamModel, direct := strings.Cut(requestedModel, "/")
	if !direct && s.defaultProvider != "" {
		providerName, upstreamModel, direct = s.defaultProvider, requestedModel, true
	}
	if !direct {
		return nil, core.ErrNoRoute
	}
	definition, exists := s.providers[providerName]
	if !exists || !definition.Configured || strings.TrimSpace(upstreamModel) == "" {
		return nil, core.ErrNoRoute
	}
	candidate := candidateFromDefinition(providerName, providerName+"/default", upstreamModel, definition.Protocol, definition, RouteConfig{HealthScore: 1, ConcurrencyLimit: 8})
	return s.applyRuntimeState([]routing.Candidate{candidate}), nil
}

func candidateFromDefinition(providerName, accountID, model string, protocol canonical.Protocol, definition providerDefinition, configured RouteConfig) routing.Candidate {
	health := configured.HealthScore
	if health <= 0 {
		health = 1
	}
	limit := configured.ConcurrencyLimit
	if limit <= 0 {
		limit = 8
	}
	region := strings.TrimSpace(configured.Region)
	if region == "" {
		region = "global"
	}
	return routing.Candidate{
		Provider:         providerName,
		AccountID:        accountID,
		Model:            strings.TrimSpace(model),
		Protocol:         protocol,
		Capabilities:     definition.Adapter.Capabilities(model),
		Region:           region,
		EstimatedCost:    configured.EstimatedCost,
		HealthScore:      health,
		QueueDepth:       configured.QueueDepth,
		ConcurrencyLimit: limit,
		Available:        true,
	}
}

func cloneCandidates(values []routing.Candidate) []routing.Candidate {
	out := make([]routing.Candidate, len(values))
	for index, candidate := range values {
		candidate.Capabilities = candidate.Capabilities.Clone()
		out[index] = candidate
	}
	return out
}

func validateProtocol(protocol canonical.Protocol) error {
	switch protocol {
	case canonical.ProtocolOpenAIResponses, canonical.ProtocolOpenAIChat, canonical.ProtocolAnthropicMessages, canonical.ProtocolGeminiGenerate:
		return nil
	default:
		return fmt.Errorf("%w: unsupported provider protocol %s", core.ErrInvalidConfiguration, protocol)
	}
}
