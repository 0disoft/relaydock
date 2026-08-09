package contextpack

import "time"

type Pack struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenantId,omitempty"`
	ProjectID       string          `json:"projectId,omitempty"`
	RepositoryRoot  string          `json:"repositoryRoot"`
	Revision        string          `json:"revision"`
	WorkingTreeHash string          `json:"workingTreeHash"`
	Objective       string          `json:"objective"`
	SuccessCriteria []string        `json:"successCriteria,omitempty"`
	Evidence        []Evidence      `json:"evidence"`
	Attempts        []Attempt       `json:"attempts,omitempty"`
	OpenQuestions   []string        `json:"openQuestions,omitempty"`
	RedactionReport RedactionReport `json:"redactionReport"`
	EstimatedBytes  int64           `json:"estimatedBytes"`
	EstimatedTokens int64           `json:"estimatedTokens"`
	ExcludedFiles   int             `json:"excludedFiles"`
	CreatedAt       time.Time       `json:"createdAt"`
	ExpiresAt       time.Time       `json:"expiresAt"`
}

type Evidence struct {
	Reference string  `json:"reference"`
	Digest    string  `json:"digest"`
	Reason    string  `json:"reason"`
	Content   string  `json:"content,omitempty"`
	Bytes     int64   `json:"bytes"`
	Score     float64 `json:"score"`
}

type Attempt struct {
	Approach string `json:"approach"`
	Result   string `json:"result"`
	Reason   string `json:"reason"`
}

type RedactionReport struct {
	RemovedFiles    int                `json:"removedFiles"`
	RemovedSegments int                `json:"removedSegments"`
	Findings        []RedactionFinding `json:"findings,omitempty"`
}

type RedactionFinding struct {
	Rule      string `json:"rule"`
	Reference string `json:"reference"`
	Severity  string `json:"severity"`
}

type BuildRequest struct {
	TenantID        string        `json:"tenantId,omitempty"`
	ProjectID       string        `json:"projectId,omitempty"`
	RepositoryRoot  string        `json:"repositoryRoot"`
	Objective       string        `json:"objective"`
	SuccessCriteria []string      `json:"successCriteria,omitempty"`
	CandidatePaths  []string      `json:"candidatePaths,omitempty"`
	MaximumBytes    int64         `json:"maximumBytes,omitempty"`
	Attempts        []Attempt     `json:"attempts,omitempty"`
	OpenQuestions   []string      `json:"openQuestions,omitempty"`
	TTL             time.Duration `json:"-"`
}

type Selection struct {
	Evidence      []Evidence `json:"evidence"`
	ExcludedFiles int        `json:"excludedFiles"`
	SelectedBytes int64      `json:"selectedBytes"`
}
