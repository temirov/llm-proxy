package proxy

import "strings"

// ResolveModelSpecification returns capabilities using the shared capability table.
func ResolveModelSpecification(modelIdentifier string) modelCapabilities {
	lower := strings.ToLower(strings.TrimSpace(modelIdentifier))
	if capabilities, found := lookupModelCapabilities(lower); found {
		return capabilities
	}
	return modelCapabilities{apiFlavor: apiFlavorResponses}
}
