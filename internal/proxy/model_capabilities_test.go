package proxy_test

import (
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
)

const (
	messageFieldsMismatch = "model %s fields=%v want=%v"
	keyModel              = "model"
	keyInput              = "input"
	keyMaxOutputTokens    = "max_output_tokens"
	keyTemperature        = "temperature"
	keyTools              = "tools"
	keyToolChoice         = "tool_choice"
)

// TestResolveModelPayloadSchema verifies that payload schemas are returned for every model.
func TestResolveModelPayloadSchema(testFramework *testing.T) {
	testCases := []struct {
		modelIdentifier string
		expectFields    []string
	}{
		{proxy.ModelNameGPT4oMini, []string{keyModel, keyInput, keyMaxOutputTokens, keyTemperature}},
		{proxy.ModelNameGPT4o, []string{keyModel, keyInput, keyMaxOutputTokens, keyTemperature, keyTools, keyToolChoice}},
		{proxy.ModelNameGPT41, []string{keyModel, keyInput, keyMaxOutputTokens, keyTemperature, keyTools, keyToolChoice}},
		{proxy.ModelNameGPT5Mini, []string{keyModel, keyInput, keyMaxOutputTokens}},
		{proxy.ModelNameGPT5, []string{keyModel, keyInput, keyMaxOutputTokens, keyTools, keyToolChoice}},
	}
	for _, testCase := range testCases {
		payloadSchema := proxy.ResolveModelPayloadSchema(testCase.modelIdentifier)
		if !equalSlices(payloadSchema.AllowedRequestFields, testCase.expectFields) {
			testFramework.Fatalf(messageFieldsMismatch, testCase.modelIdentifier, payloadSchema.AllowedRequestFields, testCase.expectFields)
		}
	}
}

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
