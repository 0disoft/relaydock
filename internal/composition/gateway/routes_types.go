package gateway

import (
	"sync"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
	"github.com/your-org/ai-runtime-gateway/internal/routing"
)

type RouteConfig struct {
	Provider         string             `json:"provider"`
	AccountID        string             `json:"accountId,omitempty"`
	Model            string             `json:"model"`
	Protocol         canonical.Protocol `json:"protocol,omitempty"`
	Region           string             `json:"region,omitempty"`
	EstimatedCost    int64              `json:"estimatedCost,omitempty"`
	HealthScore      float64            `json:"healthScore,omitempty"`
	QueueDepth       int                `json:"queueDepth,omitempty"`
	ConcurrencyLimit int                `json:"concurrencyLimit,omitempty"`
	Disabled         bool               `json:"disabled,omitempty"`
}

type providerDefinition struct {
	Adapter    provider.Adapter
	Protocol   canonical.Protocol
	Configured bool
}

type CandidateSource struct {
	routeMu         sync.RWMutex
	routes          map[string][]routing.Candidate
	providers       map[string]providerDefinition
	defaultProvider string

	mu                      sync.Mutex
	states                  map[string]candidateState
	now                     func() time.Time
	requireFreshSnapshot    bool
	allowDirectRouting      bool
	maximumRuntimeStates    int
	runtimeStateTTL         time.Duration
	snapshotRevision        int64
	snapshotExpiresAt       time.Time
	snapshotPriceRevisionID string
}

type SnapshotStatus struct {
	Required           bool      `json:"required"`
	AllowDirectRouting bool      `json:"allowDirectRouting"`
	Revision           int64     `json:"revision"`
	ExpiresAt          time.Time `json:"expiresAt,omitempty"`
	PriceRevisionID    string    `json:"priceRevisionId,omitempty"`
	Ready              bool      `json:"ready"`
}

type candidateState struct {
	ConsecutiveFailures int
	HealthMultiplier    float64
	CooldownUntil       time.Time
	LastLatency         time.Duration
	LastSeen            time.Time
}

const (
	defaultMaximumRuntimeStates = 4_096
	defaultRuntimeStateTTL      = 24 * time.Hour
)
