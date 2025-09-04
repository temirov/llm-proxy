package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
)

// TestOpenAIResponsesRetries verifies that the proxy retries upstream server errors and ultimately returns HTTP 502.
func TestOpenAIResponsesRetries(testingInstance *testing.T) {
	responsesAPICallCount := 0
	openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch httpRequest.URL.Path {
		case integrationModelsPath:
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(responseWriter, integrationModelListBody)
		case integrationResponsesPath:
			responsesAPICallCount++
			responseWriter.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(responseWriter, httpRequest)
		}
	}))
	testingInstance.Cleanup(openAIServer.Close)

	applicationServer := newIntegrationServer(testingInstance, openAIServer)
	requestURL := applicationServer.URL + "?prompt=ping&key=" + integrationServiceSecret
	httpResponse, requestError := http.Get(requestURL)
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusBadGateway {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		testingInstance.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBody))
	}
	expectedCalls := int(proxy.ResponsesMaxRetries) + 1
	if responsesAPICallCount != expectedCalls {
		testingInstance.Fatalf("calls=%d want=%d", responsesAPICallCount, expectedCalls)
	}
}
