package proxy

import "testing"

// TestResolveModelSpecification verifies that model capabilities come from the capability table.
func TestResolveModelSpecification(t *testing.T) {
	testCases := []struct {
		modelIdentifier   string
		expectTemperature bool
		expectWebSearch   bool
	}{
		{modelPrefixGPT4o, true, true},
		{modelPrefixGPT5Mini, false, false},
	}
	for _, tc := range testCases {
		capability := resolveModelSpecification(tc.modelIdentifier)
		if capability.supportsTemperature != tc.expectTemperature {
			t.Fatalf("model %s temperature=%v want=%v", tc.modelIdentifier, capability.supportsTemperature, tc.expectTemperature)
		}
		if capability.supportsWebSearch != tc.expectWebSearch {
			t.Fatalf("model %s webSearch=%v want=%v", tc.modelIdentifier, capability.supportsWebSearch, tc.expectWebSearch)
		}
	}
}
