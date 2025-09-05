package proxy_test

import (
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
)

const (
	messageTemperatureMismatch = "model %s temperature=%v want=%v"
	messageToolsMismatch       = "model %s tools=%v want=%v"
	messageToolChoiceMismatch  = "model %s toolChoice=%v want=%v"
)

// TestResolveModelPayloadSchema verifies that payload schemas are returned for every model.
func TestResolveModelPayloadSchema(testFramework *testing.T) {
	testCases := []struct {
		modelIdentifier   string
		expectTemperature bool
		expectTools       bool
		expectToolChoice  bool
	}{
		{proxy.ModelNameGPT4oMini, true, false, false},
		{proxy.ModelNameGPT4o, true, true, true},
		{proxy.ModelNameGPT41, true, true, true},
		{proxy.ModelNameGPT5Mini, false, false, false},
		{proxy.ModelNameGPT5, false, true, true},
	}
	for _, testCase := range testCases {
		payloadSchema := proxy.ResolveModelPayloadSchema(testCase.modelIdentifier)
		if payloadSchema.Temperature != testCase.expectTemperature {
			testFramework.Fatalf(messageTemperatureMismatch, testCase.modelIdentifier, payloadSchema.Temperature, testCase.expectTemperature)
		}
		if payloadSchema.Tools != testCase.expectTools {
			testFramework.Fatalf(messageToolsMismatch, testCase.modelIdentifier, payloadSchema.Tools, testCase.expectTools)
		}
		if payloadSchema.ToolChoice != testCase.expectToolChoice {
			testFramework.Fatalf(messageToolChoiceMismatch, testCase.modelIdentifier, payloadSchema.ToolChoice, testCase.expectToolChoice)
		}
	}
}

// TestResolveModelPayloadSchemaUnknown verifies that unknown models return an empty schema.
func TestResolveModelPayloadSchemaUnknown(testFramework *testing.T) {
	unknownSchema := proxy.ResolveModelPayloadSchema("unknown-model")
	if unknownSchema != (proxy.ModelPayloadSchema{}) {
		testFramework.Fatalf("unknown model returned non-empty schema")
	}
}
