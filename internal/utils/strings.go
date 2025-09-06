package utils

import (
	"strings"

	"github.com/temirov/llm-proxy/internal/constants"
)

// IsBlank reports whether a string is empty or whitespace-only.
func IsBlank(value string) bool {
	return strings.TrimSpace(value) == constants.EmptyString
}

// HasAnyPrefix reports whether value starts with any of the given prefixes (case-insensitive).
func HasAnyPrefix(value string, prefixes ...string) bool {
	lower := strings.ToLower(value)
	for _, candidatePrefix := range prefixes {
		if strings.HasPrefix(lower, strings.ToLower(candidatePrefix)) {
			return true
		}
	}
	return false
}

// GetString returns a string value from the provided container for the specified field.
func GetString(container map[string]any, field string) string {
	if container == nil {
		return constants.EmptyString
	}
	if rawValue, present := container[field]; present {
		if castValue, isString := rawValue.(string); isString {
			return castValue
		}
	}
	return constants.EmptyString
}
