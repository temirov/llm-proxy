package proxy

import "strings"

const (
	// API flavor constants.
	apiFlavorResponses = "responses"
	// Model prefix constants.
	modelPrefixGPT4oMini = "gpt-4o-mini"
	modelPrefixGPT4o     = "gpt-4o"
	modelPrefixGPT41     = "gpt-4.1"
	modelPrefixGPT5Mini  = "gpt-5-mini"
	modelPrefixGPT5      = "gpt-5"
	// modelNameSeparator marks the boundary between model prefix and variant.
	modelNameSeparator = "-"
)

// ModelCapabilities describes the features supported by a model.
type ModelCapabilities struct {
	apiFlavor           string
	supportsWebSearch   bool
	supportsTemperature bool
}

// SupportsWebSearch reports whether the model allows web search.
func (capabilities ModelCapabilities) SupportsWebSearch() bool {
	return capabilities.supportsWebSearch
}

// SupportsTemperature reports whether the model allows setting temperature.
func (capabilities ModelCapabilities) SupportsTemperature() bool {
	return capabilities.supportsTemperature
}

// capabilitiesByPrefix defines known capabilities for recognized model prefixes.
var capabilitiesByPrefix = map[string]ModelCapabilities{
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
	modelPrefixGPT5: {
		apiFlavor:           apiFlavorResponses,
		supportsWebSearch:   true,
		supportsTemperature: true,
	},
}

// lookupModelCapabilities finds capabilities for the given model identifier.
func lookupModelCapabilities(modelIdentifier string) (ModelCapabilities, bool) {
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
	return ModelCapabilities{}, false
}

// mustRejectWebSearchAtIngress lists models for which web search requests should fail fast.
func mustRejectWebSearchAtIngress(modelIdentifier string) bool {
	normalizedModelID := strings.ToLower(strings.TrimSpace(modelIdentifier))
	return strings.HasPrefix(normalizedModelID, modelPrefixGPT4oMini)
}

// ResolveModelSpecification returns capabilities using the shared capability table.
func ResolveModelSpecification(modelIdentifier string) ModelCapabilities {
	lower := strings.ToLower(strings.TrimSpace(modelIdentifier))
	if capabilities, found := lookupModelCapabilities(lower); found {
		return capabilities
	}
	return ModelCapabilities{apiFlavor: apiFlavorResponses}
}
