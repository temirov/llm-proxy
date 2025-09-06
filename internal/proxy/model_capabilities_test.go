package proxy_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
)

// TestResolveModelPayloadSchema verifies that payload schemas are returned for every model.
func TestResolveModelPayloadSchema(testFramework *testing.T) {
	testCases := []struct {
		modelIdentifier string
		expectFields    []string
	}{
		{proxy.ModelNameGPT4oMini, []string{"model", "input", "max_output_tokens", "temperature"}},
		{proxy.ModelNameGPT4o, []string{"model", "input", "max_output_tokens", "temperature", "tools", "tool_choice"}},
		{proxy.ModelNameGPT41, []string{"model", "input", "max_output_tokens", "temperature", "tools", "tool_choice"}},
		{proxy.ModelNameGPT5Mini, []string{"model", "input", "max_output_tokens"}},
		{proxy.ModelNameGPT5, []string{"model", "input", "max_output_tokens", "tools", "tool_choice", "reasoning"}},
	}
	for _, testCase := range testCases {
		payloadSchema := proxy.ResolveModelPayloadSchema(testCase.modelIdentifier)
		if !equalSlices(payloadSchema.AllowedRequestFields, testCase.expectFields) {
			testFramework.Fatalf("model %s fields=%v want=%v", testCase.modelIdentifier, payloadSchema.AllowedRequestFields, testCase.expectFields)
		}
	}
}

// TestBuildRequestPayload verifies the correct payload structure is built for each model.
func TestBuildRequestPayload(t *testing.T) {
	const prompt = "hello"

	testCases := []struct {
		name              string
		modelIdentifier   string
		webSearchEnabled  bool
		expectTemperature bool
		expectTools       bool
	}{
		{
			name:              "GPT-5 with web search",
			modelIdentifier:   proxy.ModelNameGPT5,
			webSearchEnabled:  true,
			expectTemperature: false,
			expectTools:       true,
		},
		{
			name:              "GPT-5 without web search",
			modelIdentifier:   proxy.ModelNameGPT5,
			webSearchEnabled:  false,
			expectTemperature: false,
			expectTools:       false,
		},
		{
			name:              "GPT-4o with web search",
			modelIdentifier:   proxy.ModelNameGPT4o,
			webSearchEnabled:  true,
			expectTemperature: true,
			expectTools:       true,
		},
		{
			name:              "GPT-4o-mini (no tools)",
			modelIdentifier:   proxy.ModelNameGPT4oMini,
			webSearchEnabled:  true, // Ignored
			expectTemperature: true,
			expectTools:       false,
		},
		{
			name:              "GPT-5-mini (base only)",
			modelIdentifier:   proxy.ModelNameGPT5Mini,
			webSearchEnabled:  true, // Ignored
			expectTemperature: false,
			expectTools:       false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			payload := proxy.BuildRequestPayload(testCase.modelIdentifier, prompt, testCase.webSearchEnabled)
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("Failed to marshal payload: %v", err)
			}
			payloadJSON := string(payloadBytes)

			if testCase.expectTemperature != strings.Contains(payloadJSON, `"temperature"`) {
				t.Errorf("Mismatch in 'temperature' field presence. Got: %s, Want presence: %v", payloadJSON, testCase.expectTemperature)
			}
			if testCase.expectTools != strings.Contains(payloadJSON, `"tools"`) {
				t.Errorf("Mismatch in 'tools' field presence. Got: %s, Want presence: %v", payloadJSON, testCase.expectTools)
			}
			if testCase.expectTools != strings.Contains(payloadJSON, `"tool_choice"`) {
				t.Errorf("Mismatch in 'tool_choice' field presence. Got: %s, Want presence: %v", payloadJSON, testCase.expectTools)
			}
		})
	}
}

// equalSlices reports whether both string slices contain the same elements in
// the same order.
func equalSlices(first []string, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}
