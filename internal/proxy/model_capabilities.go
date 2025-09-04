package proxy

import "strings"

// modelCapabilities describes the features supported by a model.
//
// The flags indicate the API flavor and whether optional features such as web
// search and temperature are available.
type modelCapabilities struct {
	apiFlavor           string
	supportsWebSearch   bool
	supportsTemperature bool
}

// modelPattern associates a model prefix with its capabilities.
//
// When a model identifier matches the prefix, the corresponding capabilities
// are applied.
type modelPattern struct {
	prefix       string
	capabilities modelCapabilities
}

var capabilityTable = []modelPattern{
	{prefix: "gpt-4o-mini", capabilities: modelCapabilities{apiFlavor: apiResponses, supportsWebSearch: false, supportsTemperature: true}},
	{prefix: "gpt-4o", capabilities: modelCapabilities{apiFlavor: apiResponses, supportsWebSearch: true, supportsTemperature: true}},
	{prefix: "gpt-4.1", capabilities: modelCapabilities{apiFlavor: apiResponses, supportsWebSearch: true, supportsTemperature: true}},
	{prefix: "gpt-5-mini", capabilities: modelCapabilities{apiFlavor: apiResponses, supportsWebSearch: false, supportsTemperature: false}},
}

// lookupModelCapabilities finds the capability configuration for the provided
// model identifier. It returns the capabilities and true if a match is found.
func lookupModelCapabilities(modelIdentifier string) (modelCapabilities, bool) {
	for _, entry := range capabilityTable {
		if modelIdentifier == entry.prefix || strings.HasPrefix(modelIdentifier, entry.prefix) {
			return entry.capabilities, true
		}
	}
	return modelCapabilities{}, false
}

// modelSupportsWebSearch reports whether the model allows web search.
func modelSupportsWebSearch(modelIdentifier string) bool {
	if capabilities, ok := lookupModelCapabilities(modelIdentifier); ok {
		return capabilities.supportsWebSearch
	}
	return false
}

// mustRejectWebSearchAtIngress lists models for which we hard-fail if web_search=1 is requested.
// Keep 4o-mini strict to save upstream calls, while allowing other minis to be handled adaptively.
func mustRejectWebSearchAtIngress(modelIdentifier string) bool {
	normalizedModelID := strings.ToLower(strings.TrimSpace(modelIdentifier))
	return strings.HasPrefix(normalizedModelID, "gpt-4o-mini")
}
