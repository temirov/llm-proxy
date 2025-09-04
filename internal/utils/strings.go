package utils

import "strings"

// IsBlank reports whether a string is empty or whitespace-only.
func IsBlank(value string) bool {
	return strings.TrimSpace(value) == ""
}

// HasAnyPrefix reports whether value starts with any of the given prefixes (case-insensitive).
func HasAnyPrefix(value string, prefixes ...string) bool {
	lower := strings.ToLower(value)
	for _, p := range prefixes {
		if strings.HasPrefix(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}
