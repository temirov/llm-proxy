package proxy

import "strings"

// resolveModelSpecification returns capabilities using the shared capability table.
func resolveModelSpecification(modelIdentifier string) modelCapabilities {
	lower := strings.ToLower(strings.TrimSpace(modelIdentifier))
	if capabilities, found := lookupModelCapabilities(lower); found {
		return capabilities
	}
	return modelCapabilities{apiFlavor: apiFlavorResponses}
}
