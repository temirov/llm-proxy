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

// modelCapabilities describes the features supported by a model.
type modelCapabilities struct {
	apiFlavor           string
	supportsWebSearch   bool
	supportsTemperature bool
}

// modelCapabilityPattern maps a model prefix to its capabilities.
type modelCapabilityPattern struct {
	prefix       string
	capabilities modelCapabilities
}

// capabilitiesTable defines known capabilities for recognized model prefixes.
var capabilitiesTable = []modelCapabilityPattern{
	{prefix: modelPrefixGPT4oMini, capabilities: modelCapabilities{apiFlavor: apiFlavorResponses, supportsWebSearch: false, supportsTemperature: true}},
	{prefix: modelPrefixGPT4o, capabilities: modelCapabilities{apiFlavor: apiFlavorResponses, supportsWebSearch: true, supportsTemperature: true}},
	{prefix: modelPrefixGPT41, capabilities: modelCapabilities{apiFlavor: apiFlavorResponses, supportsWebSearch: true, supportsTemperature: true}},
	{prefix: modelPrefixGPT5Mini, capabilities: modelCapabilities{apiFlavor: apiFlavorResponses, supportsWebSearch: false, supportsTemperature: false}},
}

// lookupModelCapabilities finds capabilities for the given model identifier.
func lookupModelCapabilities(modelIdentifier string) (modelCapabilities, bool) {
	for _, entry := range capabilitiesTable {
		if modelIdentifier == entry.prefix || strings.HasPrefix(modelIdentifier, entry.prefix) {
			return entry.capabilities, true
		}
	}
	return modelCapabilities{}, false
}

// mustRejectWebSearchAtIngress lists models for which web search requests should fail fast.
func mustRejectWebSearchAtIngress(modelIdentifier string) bool {
	normalizedModelID := strings.ToLower(strings.TrimSpace(modelIdentifier))
	return strings.HasPrefix(normalizedModelID, modelPrefixGPT4oMini)
}
