package routing

import (
	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
	"time"
)

type Candidate struct {
	Provider         string
	AccountID        string
	Model            string
	Protocol         canonical.Protocol
	Capabilities     canonical.CapabilitySet
	Region           string
	EstimatedCost    int64
	HealthScore      float64
	QueueDepth       int
	ConcurrencyLimit int
	SessionAffinity  bool
	CacheAffinity    bool
	Available        bool
}
type Decision struct {
	Candidate Candidate
	Score     float64
	ExpiresAt time.Time
	Reasons   []string
}
type Request struct {
	Requirements     canonical.CapabilityRequirements
	PolicyID         string
	SessionID        string
	MaximumCost      int64
	AllowedProviders []string
	AllowedRegions   []string
}
