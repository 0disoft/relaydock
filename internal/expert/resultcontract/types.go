package resultcontract

type Result struct {
	Decision                string         `json:"decision"`
	Confidence              float64        `json:"confidence"`
	Assumptions             []string       `json:"assumptions"`
	CriticalFindings        []Finding      `json:"criticalFindings"`
	RecommendedArchitecture map[string]any `json:"recommendedArchitecture"`
	RejectedAlternatives    []Alternative  `json:"rejectedAlternatives"`
	FailureScenarios        []string       `json:"failureScenarios"`
	MigrationOrder          []string       `json:"migrationOrder"`
	VerificationPlan        []string       `json:"verificationPlan"`
	UnresolvedQuestions     []string       `json:"unresolvedQuestions"`
	EvidenceReferences      []string       `json:"evidenceReferences"`
}

type Finding struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Evidence string `json:"evidence"`
}

type Alternative struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}
