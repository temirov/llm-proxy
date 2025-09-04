package utils_test

import (
	"testing"

	"github.com/temirov/llm-proxy/internal/utils"
)

const (
	secretEmpty         = ""
	secretABC           = "abc"
	secretLLMProxy      = "llm-proxy"
	fingerprintEmpty    = "e3b0c442"
	fingerprintABC      = "ba7816bf"
	fingerprintLLMProxy = "c30d6864"
)

type fingerprintTestDefinition struct {
	testName      string
	secretValue   string
	expectedValue string
}

// TestFingerprint_OutputMatchesExpected verifies that Fingerprint returns the expected values for a variety of inputs.
func TestFingerprint_OutputMatchesExpected(testingInstance *testing.T) {
	testCases := []fingerprintTestDefinition{
		{testName: "empty string", secretValue: secretEmpty, expectedValue: fingerprintEmpty},
		{testName: "short string", secretValue: secretABC, expectedValue: fingerprintABC},
		{testName: "longer string", secretValue: secretLLMProxy, expectedValue: fingerprintLLMProxy},
	}
	for _, currentTestCase := range testCases {
		testingInstance.Run(currentTestCase.testName, func(nestedTestingInstance *testing.T) {
			actualFingerprint := utils.Fingerprint(currentTestCase.secretValue)
			if actualFingerprint != currentTestCase.expectedValue {
				nestedTestingInstance.Fatalf("fingerprint=%s expected=%s", actualFingerprint, currentTestCase.expectedValue)
			}
		})
	}
}
