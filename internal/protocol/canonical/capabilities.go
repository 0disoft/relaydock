package canonical

import "strings"

type Capability string

const (
	CapabilityTextInput        Capability = "text.input"
	CapabilityImageInput       Capability = "image.input"
	CapabilityToolCalling      Capability = "tool.calling"
	CapabilityParallelTools    Capability = "tool.parallel"
	CapabilityStructuredOutput Capability = "output.structured"
	CapabilityReasoning        Capability = "reasoning"
	CapabilityContinuation     Capability = "continuation"
	CapabilityPromptCaching    Capability = "prompt.caching"
)

type CapabilityRequirements struct {
	Required             []Capability
	MaximumContextTokens int64
	MaximumOutputTokens  int64
	DataResidency        []string
}

type CapabilitySet map[Capability]bool

func (s CapabilitySet) Supports(requirements CapabilityRequirements) bool {
	for _, c := range requirements.Required {
		if !s[c] {
			return false
		}
	}
	return true
}
func (s CapabilitySet) Clone() CapabilitySet {
	out := make(CapabilitySet, len(s))
	for k, v := range s {
		out[k] = v
	}
	return out
}
func (r CapabilityRequirements) AllowsRegion(region string) bool {
	if len(r.DataResidency) == 0 {
		return true
	}
	for _, allowed := range r.DataResidency {
		if strings.EqualFold(strings.TrimSpace(allowed), strings.TrimSpace(region)) {
			return true
		}
	}
	return false
}
