package proxy_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
)

const (
	marshalPayloadErrorFormat        = "Failed to marshal payload: %v"
	temperatureFieldPresenceMismatch = "Mismatch in 'temperature' field presence. Got: %s, Want presence: %v"
	toolsFieldPresenceMismatch       = "Mismatch in 'tools' field presence. Got: %s, Want presence: %v"
	toolChoiceFieldPresenceMismatch  = "Mismatch in 'tool_choice' field presence. Got: %s, Want presence: %v"
	reasoningFieldPresenceMismatch   = "Mismatch in 'reasoning' field presence. Got: %s, Want presence: %v"
	reasoningFieldJSONFragment       = `"reasoning"`
	modelFieldsMismatchFormat        = "model %s fields=%v want=%v"
	promptValue                      = "hello"
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
			testFramework.Fatalf(modelFieldsMismatchFormat, testCase.modelIdentifier, payloadSchema.AllowedRequestFields, testCase.expectFields)
		}
	}
}

// TestBuildRequestPayload verifies the correct payload structure is built for each model.
func TestBuildRequestPayload(testFramework *testing.T) {
	testCases := []struct {
		name              string
		modelIdentifier   string
		webSearchEnabled  bool
		expectTemperature bool
		expectTools       bool
		expectReasoning   bool
	}{
		{
			name:              "GPT-5 with web search",
			modelIdentifier:   proxy.ModelNameGPT5,
			webSearchEnabled:  true,
			expectTemperature: false,
			expectTools:       true,
			expectReasoning:   true,
		},
		{
			name:              "GPT-5 without web search",
			modelIdentifier:   proxy.ModelNameGPT5,
			webSearchEnabled:  false,
			expectTemperature: false,
			expectTools:       false,
			expectReasoning:   false,
		},
		{
			name:              "GPT-4o with web search",
			modelIdentifier:   proxy.ModelNameGPT4o,
			webSearchEnabled:  true,
			expectTemperature: true,
			expectTools:       true,
			expectReasoning:   false,
		},
		{
			name:              "GPT-4o-mini (no tools)",
			modelIdentifier:   proxy.ModelNameGPT4oMini,
			webSearchEnabled:  true, // Ignored
			expectTemperature: true,
			expectTools:       false,
			expectReasoning:   false,
		},
		{
			name:              "GPT-5-mini (base only)",
			modelIdentifier:   proxy.ModelNameGPT5Mini,
			webSearchEnabled:  true, // Ignored
			expectTemperature: false,
			expectTools:       false,
			expectReasoning:   false,
		},
	}

	for _, testCase := range testCases {
		testFramework.Run(testCase.name, func(subTestFramework *testing.T) {
			payload := proxy.BuildRequestPayload(testCase.modelIdentifier, promptValue, testCase.webSearchEnabled, proxy.DefaultMaxOutputTokens)
			payloadBytes, marshalError := json.Marshal(payload)
			if marshalError != nil {
				subTestFramework.Fatalf(marshalPayloadErrorFormat, marshalError)
			}
			payloadJSON := string(payloadBytes)

			if testCase.expectTemperature != strings.Contains(payloadJSON, `"temperature"`) {
				subTestFramework.Errorf(temperatureFieldPresenceMismatch, payloadJSON, testCase.expectTemperature)
			}
			if testCase.expectTools != strings.Contains(payloadJSON, `"tools"`) {
				subTestFramework.Errorf(toolsFieldPresenceMismatch, payloadJSON, testCase.expectTools)
			}
			if testCase.expectTools != strings.Contains(payloadJSON, `"tool_choice"`) {
				subTestFramework.Errorf(toolChoiceFieldPresenceMismatch, payloadJSON, testCase.expectTools)
			}
			reasoningFieldPresent := strings.Contains(payloadJSON, reasoningFieldJSONFragment)
			if reasoningFieldPresent != testCase.expectReasoning {
				subTestFramework.Errorf(reasoningFieldPresenceMismatch, payloadJSON, testCase.expectReasoning)
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
