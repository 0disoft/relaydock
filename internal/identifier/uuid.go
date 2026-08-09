// Package identifier contains dependency-free validation helpers for IDs that
// cross process and storage boundaries. It intentionally validates syntax only;
// ownership and existence checks belong to the caller's repository layer.
package identifier

import "strings"

// IsUUID reports whether value is a canonical 36-character UUID string.
// It accepts upper- or lower-case hexadecimal digits and rejects whitespace,
// braces, URN prefixes, truncated values, and non-canonical hyphen positions.
func IsUUID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !isHex(char) {
			return false
		}
	}
	return true
}

func isHex(value rune) bool {
	return (value >= '0' && value <= '9') ||
		(value >= 'a' && value <= 'f') ||
		(value >= 'A' && value <= 'F')
}
