package proxy

import "strings"

// API flavor constants.
const apiFlavorResponses = "responses"

// Model prefix constants.
const (
	modelPrefixGPT4oMini = "gpt-4o-mini"
	modelPrefixGPT4o     = "gpt-4o"
	modelPrefixGPT41     = "gpt-4.1"
	modelPrefixGPT5Mini  = "gpt-5-mini"
)

// modelNameSeparator marks the boundary between model prefix and variant.
const modelNameSeparator = "-"

// modelCapabilities describes the features supported by a model.
type modelCapabilities struct {
	apiFlavor           string
	supportsWebSearch   bool
	supportsTemperature bool
}

// SupportsWebSearch reports whether the model allows web search.
func (capabilities modelCapabilities) SupportsWebSearch() bool {
	return capabilities.supportsWebSearch
}

// SupportsTemperature reports whether the model allows setting temperature.
func (capabilities modelCapabilities) SupportsTemperature() bool {
	return capabilities.supportsTemperature
}

// capabilitiesByPrefix defines known capabilities for recognized model prefixes.
var capabilitiesByPrefix = map[string]modelCapabilities{
	modelPrefixGPT4oMini: {
		apiFlavor:           apiFlavorResponses,
		supportsWebSearch:   false,
		supportsTemperature: true,
	},
	modelPrefixGPT4o: {
		apiFlavor:           apiFlavorResponses,
		supportsWebSearch:   true,
		supportsTemperature: true,
	},
	modelPrefixGPT41: {
		apiFlavor:           apiFlavorResponses,
		supportsWebSearch:   true,
		supportsTemperature: true,
	},
	modelPrefixGPT5Mini: {
		apiFlavor:           apiFlavorResponses,
		supportsWebSearch:   false,
		supportsTemperature: false,
	},
}

// lookupModelCapabilities finds capabilities for the given model identifier.
func lookupModelCapabilities(modelIdentifier string) (modelCapabilities, bool) {
	for {
		if capabilities, found := capabilitiesByPrefix[modelIdentifier]; found {
			return capabilities, true
		}
		lastSeparatorIndex := strings.LastIndex(modelIdentifier, modelNameSeparator)
		if lastSeparatorIndex == -1 {
			break
		}
		modelIdentifier = modelIdentifier[:lastSeparatorIndex]
	}
	return modelCapabilities{}, false
}

// mustRejectWebSearchAtIngress lists models for which web search requests should fail fast.
func mustRejectWebSearchAtIngress(modelIdentifier string) bool {
	normalizedModelID := strings.ToLower(strings.TrimSpace(modelIdentifier))
	return strings.HasPrefix(normalizedModelID, modelPrefixGPT4oMini)
}
