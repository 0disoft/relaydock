package redaction

import (
	"context"
	"math"
	"path/filepath"
	"regexp"
	"strings"
)

type Finding struct {
	Rule                string
	Offset, Length      int
	Severity, Reference string
}
type Result struct {
	Sanitized []byte
	Findings  []Finding
}
type Redactor interface {
	Redact(context.Context, string, []byte) (Result, error)
}
type Scanner struct {
	rules          []Rule
	entropyPattern *regexp.Regexp
}

func NewScanner(rules ...Rule) *Scanner {
	return &Scanner{rules: rules, entropyPattern: regexp.MustCompile(`[A-Za-z0-9_+/=-]{32,}`)}
}
func NewDefaultScanner() (*Scanner, error) {
	rules, err := DefaultRules()
	if err != nil {
		return nil, err
	}
	return NewScanner(rules...), nil
}
func (s *Scanner) Redact(ctx context.Context, reference string, input []byte) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if deniedPath(reference) {
		return Result{Sanitized: []byte("[REDACTED_FILE]"), Findings: []Finding{{Rule: "sensitive_path", Offset: 0, Length: len(input), Severity: "critical", Reference: reference}}}, nil
	}
	sanitized := append([]byte(nil), input...)
	findings := []Finding{}
	for _, rule := range s.rules {
		matches := rule.Pattern.FindAllIndex(sanitized, -1)
		for _, m := range matches {
			findings = append(findings, Finding{Rule: rule.Name, Offset: m[0], Length: m[1] - m[0], Severity: rule.Severity, Reference: reference})
		}
		sanitized = rule.Pattern.ReplaceAll(sanitized, rule.Replacement)
	}
	matches := s.entropyPattern.FindAllIndex(sanitized, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		token := sanitized[m[0]:m[1]]
		if looksLikePlaceholder(string(token)) || shannon(token) < 4.2 {
			continue
		}
		findings = append(findings, Finding{Rule: "high_entropy_token", Offset: m[0], Length: len(token), Severity: "medium", Reference: reference})
		replacement := []byte("[REDACTED_HIGH_ENTROPY_TOKEN]")
		next := make([]byte, 0, len(sanitized)-len(token)+len(replacement))
		next = append(next, sanitized[:m[0]]...)
		next = append(next, replacement...)
		next = append(next, sanitized[m[1]:]...)
		sanitized = next
	}
	return Result{Sanitized: sanitized, Findings: findings}, nil
}
func deniedPath(reference string) bool {
	base := strings.ToLower(filepath.Base(reference))
	if base == ".env" || strings.HasPrefix(base, ".env.") || base == "credentials" || base == "credentials.json" || base == "secrets.json" || base == "id_rsa" || base == "id_ed25519" {
		return true
	}
	ext := strings.ToLower(filepath.Ext(base))
	return ext == ".pem" || ext == ".p12" || ext == ".pfx" || ext == ".key"
}
func shannon(v []byte) float64 {
	if len(v) == 0 {
		return 0
	}
	counts := map[byte]int{}
	for _, b := range v {
		counts[b]++
	}
	var h float64
	for _, n := range counts {
		p := float64(n) / float64(len(v))
		h -= p * math.Log2(p)
	}
	return h
}
func looksLikePlaceholder(v string) bool {
	lower := strings.ToLower(v)
	for _, part := range []string{"placeholder", "example", "changeme", "your_", "xxxx", "000000", "abcdef"} {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}
