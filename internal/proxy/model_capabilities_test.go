package proxy_test

import (
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
)

const (
	messageTemperatureMismatch = "model %s temperature=%v want=%v"
	messageWebSearchMismatch   = "model %s webSearch=%v want=%v"
)

// TestResolveModelSpecification verifies that model capabilities come from the capability table.
func TestResolveModelSpecification(testFramework *testing.T) {
	testCases := []struct {
		modelIdentifier   string
		expectTemperature bool
		expectWebSearch   bool
	}{
		{proxy.ModelNameGPT4o, true, true},
		{proxy.ModelNameGPT5Mini, false, false},
	}
	for _, testCase := range testCases {
		capabilities := proxy.ResolveModelSpecification(testCase.modelIdentifier)
		if capabilities.SupportsTemperature != testCase.expectTemperature {
			testFramework.Fatalf(messageTemperatureMismatch, testCase.modelIdentifier, capabilities.SupportsTemperature, testCase.expectTemperature)
		}
		if capabilities.SupportsWebSearch != testCase.expectWebSearch {
			testFramework.Fatalf(messageWebSearchMismatch, testCase.modelIdentifier, capabilities.SupportsWebSearch, testCase.expectWebSearch)
		}
	}
}
