package authorization

import "strings"

type Set map[string]struct{}

func New(scopes ...string) Set {
	set := make(Set, len(scopes))
	for _, scope := range scopes {
		if scope = strings.TrimSpace(scope); scope != "" {
			set[scope] = struct{}{}
		}
	}
	return set
}

func (s Set) Has(scope string) bool {
	_, ok := s[strings.TrimSpace(scope)]
	return ok
}

func (s Set) HasAll(scopes ...string) bool {
	for _, scope := range scopes {
		if !s.Has(scope) {
			return false
		}
	}
	return true
}
