package snapshot

import "time"

type Snapshot struct {
	SigningKeyID   string              `json:"signingKeyId,omitempty"`
	Revision       int64               `json:"revision"`
	GeneratedAt    time.Time           `json:"generatedAt"`
	ExpiresAt      time.Time           `json:"expiresAt"`
	VirtualKeys    []VirtualKey        `json:"virtualKeys"`
	Models         []ModelRoute        `json:"models"`
	ProviderRefs   []ProviderReference `json:"providerRefs"`
	PriceRevisions []PriceRevision     `json:"priceRevisions"`
	Signature      []byte              `json:"signature,omitempty"`
}

type VirtualKey struct {
	PublicID      string    `json:"publicId"`
	TenantID      string    `json:"tenantId"`
	ProjectID     string    `json:"projectId"`
	AllowedModels []string  `json:"allowedModels"`
	Scopes        []string  `json:"scopes"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

// ModelRoute retains the compact string candidate form for hand-authored
// snapshots while CandidateDetails carries the complete production routing
// contract. A data plane must prefer CandidateDetails when it is present.
type ModelRoute struct {
	VirtualModel     string           `json:"virtualModel"`
	Candidates       []string         `json:"candidates,omitempty"`
	CandidateDetails []RouteCandidate `json:"candidateDetails,omitempty"`
	PolicyID         string           `json:"policyId"`
}

// RouteCandidate is intentionally transport-neutral. Protocol is represented
// as a string here so the control-plane package does not depend on one
// particular protocol implementation package. The gateway validates it before
// swapping a snapshot into the live routing table.
type RouteCandidate struct {
	Provider         string  `json:"provider"`
	AccountID        string  `json:"accountId,omitempty"`
	Model            string  `json:"model"`
	Protocol         string  `json:"protocol,omitempty"`
	Region           string  `json:"region,omitempty"`
	EstimatedCost    int64   `json:"estimatedCost,omitempty"`
	HealthScore      float64 `json:"healthScore,omitempty"`
	QueueDepth       int     `json:"queueDepth,omitempty"`
	ConcurrencyLimit int     `json:"concurrencyLimit,omitempty"`
	Disabled         bool    `json:"disabled,omitempty"`
}

type ProviderReference struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Region   string `json:"region"`
}

type PriceRevision struct {
	ID          string    `json:"id"`
	EffectiveAt time.Time `json:"effectiveAt"`
}
