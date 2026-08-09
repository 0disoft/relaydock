package releasepack

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func LoadSizePolicy(root string, fallbackMaximum int64) (SizePolicy, error) {
	if fallbackMaximum <= 0 {
		fallbackMaximum = DefaultMaximumFileBytes
	}
	policy := SizePolicy{Version: 1, MaxBytes: fallbackMaximum}
	path := filepath.Join(root, filepath.FromSlash(fileSizeExceptionConfig))
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return policy, nil
		}
		return SizePolicy{}, fmt.Errorf("read file-size policy: %w", err)
	}
	if err := decodeStrictJSON(raw, &policy, "file-size policy"); err != nil {
		return SizePolicy{}, err
	}
	if policy.Version != 1 {
		return SizePolicy{}, fmt.Errorf("unsupported file-size policy version %d", policy.Version)
	}
	if policy.MaxBytes <= 0 {
		return SizePolicy{}, fmt.Errorf("file-size policy maxBytes must be positive")
	}
	seen := make(map[string]struct{}, len(policy.Exceptions))
	for index := range policy.Exceptions {
		exception := &policy.Exceptions[index]
		exception.Path = normalizeRelativePath(exception.Path)
		exception.Reason = strings.TrimSpace(exception.Reason)
		if exception.Path == "" || exception.Reason == "" {
			return SizePolicy{}, fmt.Errorf("file-size exception requires path and reason")
		}
		if _, exists := seen[exception.Path]; exists {
			return SizePolicy{}, fmt.Errorf("duplicate file-size exception %q", exception.Path)
		}
		seen[exception.Path] = struct{}{}
	}
	sort.Slice(policy.Exceptions, func(i, j int) bool { return policy.Exceptions[i].Path < policy.Exceptions[j].Path })
	return policy, nil
}

func (p SizePolicy) Allows(path string) (string, bool) {
	path = normalizeRelativePath(path)
	for _, exception := range p.Exceptions {
		if exception.Path == path {
			return exception.Reason, true
		}
	}
	return "", false
}
