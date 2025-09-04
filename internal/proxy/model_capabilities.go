package proxy

import "strings"

type modelCapabilities struct {
	apiFlavor           string
	supportsWebSearch   bool
	supportsTemperature bool
}

type modelPattern struct {
	prefix string
	caps   modelCapabilities
}

var capabilityTable = []modelPattern{
	{prefix: "gpt-4o-mini", caps: modelCapabilities{apiFlavor: "responses", supportsWebSearch: false, supportsTemperature: true}},
	{prefix: "gpt-4o", caps: modelCapabilities{apiFlavor: "responses", supportsWebSearch: true, supportsTemperature: true}},
	{prefix: "gpt-4.1", caps: modelCapabilities{apiFlavor: "responses", supportsWebSearch: true, supportsTemperature: true}},
	{prefix: "gpt-5-mini", caps: modelCapabilities{apiFlavor: "responses", supportsWebSearch: false, supportsTemperature: false}},
}

func lookupModelCapabilities(modelIdentifier string) (modelCapabilities, bool) {
	for _, entry := range capabilityTable {
		if modelIdentifier == entry.prefix || strings.HasPrefix(modelIdentifier, entry.prefix) {
			return entry.caps, true
		}
	}
	return modelCapabilities{}, false
}

func modelSupportsWebSearch(modelIdentifier string) bool {
	if caps, ok := lookupModelCapabilities(modelIdentifier); ok {
		return caps.supportsWebSearch
	}
	return false
}

// mustRejectWebSearchAtIngress lists models for which we hard-fail if web_search=1 is requested.
// Keep 4o-mini strict to save upstream calls, while allowing other minis to be handled adaptively.
func mustRejectWebSearchAtIngress(modelIdentifier string) bool {
	m := strings.ToLower(strings.TrimSpace(modelIdentifier))
	return strings.HasPrefix(m, "gpt-4o-mini")
}
