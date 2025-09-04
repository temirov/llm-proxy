package proxy

import "strings"

// resolveModelSpecification returns capabilities using the shared capability table.
func resolveModelSpecification(modelIdentifier string) modelCapabilities {
	lower := strings.ToLower(strings.TrimSpace(modelIdentifier))
	if capability, found := lookupModelCapabilities(lower); found {
		return capability
	}
	return modelCapabilities{apiFlavor: apiFlavorResponses}
}
