package proxy_test

import (
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
)

const (
	firstResponsesURL  = "https://one.local/v1/responses"
	secondResponsesURL = "https://two.local/v1/responses"
)

// TestEndpointsIsolation verifies that endpoint instances remain independent when used in parallel tests.
func TestEndpointsIsolation(testingInstance *testing.T) {
	testingInstance.Run("first", func(subTest *testing.T) {
		subTest.Parallel()
		endpoints := proxy.NewEndpoints()
		endpoints.SetResponsesURL(firstResponsesURL)
		if endpoints.GetResponsesURL() != firstResponsesURL {
			subTest.Fatalf("responsesURL=%s want=%s", endpoints.GetResponsesURL(), firstResponsesURL)
		}
	})
	testingInstance.Run("second", func(subTest *testing.T) {
		subTest.Parallel()
		endpoints := proxy.NewEndpoints()
		endpoints.SetResponsesURL(secondResponsesURL)
		if endpoints.GetResponsesURL() != secondResponsesURL {
			subTest.Fatalf("responsesURL=%s want=%s", endpoints.GetResponsesURL(), secondResponsesURL)
		}
	})
}
