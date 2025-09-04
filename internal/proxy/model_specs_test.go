package proxy

import "testing"

// TestResolveModelSpecification verifies that model capabilities come from the capability table.
func TestResolveModelSpecification(testFramework *testing.T) {
	testCases := []struct {
		modelIdentifier   string
		expectTemperature bool
		expectWebSearch   bool
	}{
		{modelPrefixGPT4o, true, true},
		{modelPrefixGPT5Mini, false, false},
	}
	for _, testCase := range testCases {
		capabilities := resolveModelSpecification(testCase.modelIdentifier)
		if capabilities.supportsTemperature != testCase.expectTemperature {
			testFramework.Fatalf("model %s temperature=%v want=%v", testCase.modelIdentifier, capabilities.supportsTemperature, testCase.expectTemperature)
		}
		if capabilities.supportsWebSearch != testCase.expectWebSearch {
			testFramework.Fatalf("model %s webSearch=%v want=%v", testCase.modelIdentifier, capabilities.supportsWebSearch, testCase.expectWebSearch)
		}
	}
}
