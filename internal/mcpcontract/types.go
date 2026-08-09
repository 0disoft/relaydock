package mcpcontract

import "github.com/0disoft/relaydock/internal/expert/resultcontract"

type ConsultationCreateInput struct {
	RepositoryRoot   string   `json:"repositoryRoot,omitempty" jsonschema:"absolute repository root; defaults to the desktop runtime workspace"`
	Objective        string   `json:"objective" jsonschema:"the architecture or debugging objective"`
	TaskType         string   `json:"taskType,omitempty" jsonschema:"the consultation category"`
	CandidatePaths   []string `json:"candidatePaths,omitempty" jsonschema:"repository paths that may be relevant"`
	DelegationDepth  int      `json:"delegationDepth,omitempty" jsonschema:"zero for the originating agent; recursive expert calls are rejected"`
	MaximumCostMinor int64    `json:"maximumCostMinor,omitempty" jsonschema:"hard cost ceiling in the account currency minor unit"`
}

type ConsultationCreateOutput struct {
	ConsultationID string `json:"consultationId"`
	State          string `json:"state"`
}

type ConsultationGetInput struct {
	ConsultationID string `json:"consultationId" jsonschema:"the consultation identifier"`
}

type ConsultationGetOutput struct {
	ConsultationID string                 `json:"consultationId"`
	State          string                 `json:"state"`
	Decision       string                 `json:"decision,omitempty"`
	Result         *resultcontract.Result `json:"result,omitempty"`
}

type ConsultationCancelInput struct {
	ConsultationID string `json:"consultationId" jsonschema:"the consultation identifier"`
}

type ConsultationCancelOutput struct {
	ConsultationID string `json:"consultationId"`
	State          string `json:"state"`
}

type ContextPreviewInput struct {
	RepositoryRoot string   `json:"repositoryRoot,omitempty"`
	Objective      string   `json:"objective"`
	CandidatePaths []string `json:"candidatePaths,omitempty"`
}

type ContextPreviewOutput struct {
	ContextPackID    string `json:"contextPackId"`
	IncludedFiles    int    `json:"includedFiles"`
	ExcludedFindings int    `json:"excludedFindings"`
}
