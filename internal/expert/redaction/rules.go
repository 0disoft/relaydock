package redaction

import "regexp"

type Rule struct {
	Name        string
	Pattern     *regexp.Regexp
	Replacement []byte
	Severity    string
}

func DefaultRules() ([]Rule, error) {
	specs := []struct{ name, pattern, replacement, severity string }{
		{"private_key", `(?s)-----BEGIN(?: [A-Z0-9]+)? PRIVATE KEY-----.*?-----END(?: [A-Z0-9]+)? PRIVATE KEY-----`, `[REDACTED_PRIVATE_KEY]`, `critical`},
		{"anthropic_key", `\bsk-ant-[A-Za-z0-9_-]{20,}\b`, `[REDACTED_ANTHROPIC_KEY]`, `critical`},
		{"openai_key", `\bsk-(?:proj-)?[A-Za-z0-9_-]{20,}\b`, `[REDACTED_OPENAI_KEY]`, `critical`},
		{"github_token", `\b(?:ghp|gho|ghu|ghs|github_pat)_[A-Za-z0-9_]{20,}\b`, `[REDACTED_GITHUB_TOKEN]`, `critical`},
		{"aws_access_key", `\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`, `[REDACTED_AWS_ACCESS_KEY]`, `critical`},
		{"jwt", `\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`, `[REDACTED_JWT]`, `high`},
		{"authorization_header", `(?im)^(\s*(?://|#)?\s*authorization\s*:\s*(?:bearer|basic)\s+)[^\r\n]+`, `$1[REDACTED]`, `critical`},
		{"sensitive_assignment", `(?im)\b([A-Z0-9_]*(?:SECRET|TOKEN|PASSWORD|API_KEY|PRIVATE_KEY)[A-Z0-9_]*\s*=\s*)[^\s#]+`, `$1[REDACTED]`, `high`},
		{"database_url", `(?i)\b(?:postgres(?:ql)?|mysql|mongodb(?:\+srv)?|redis)://[^\s"']+`, `[REDACTED_DATABASE_URL]`, `high`},
	}
	out := make([]Rule, 0, len(specs))
	for _, s := range specs {
		r, err := regexp.Compile(s.pattern)
		if err != nil {
			return nil, err
		}
		out = append(out, Rule{Name: s.name, Pattern: r, Replacement: []byte(s.replacement), Severity: s.severity})
	}
	return out, nil
}
