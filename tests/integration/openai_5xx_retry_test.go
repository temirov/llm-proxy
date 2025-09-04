package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	minimumExpectedCalls = 2
)

// TestOpenAIResponsesRetries verifies that the proxy retries upstream server errors and ultimately returns HTTP 504.
func TestOpenAIResponsesRetries(testingInstance *testing.T) {
	responsesAPICallCount := 0
	openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch httpRequest.URL.Path {
		case modelsPath:
			responseWriter.Header().Set(headerContentTypeKey, mimeTypeApplicationJSON)
			_, _ = io.WriteString(responseWriter, modelListBody)
		case responsesPath:
			responsesAPICallCount++
			responseWriter.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(responseWriter, httpRequest)
		}
	}))
	testingInstance.Cleanup(openAIServer.Close)

	applicationServer := newIntegrationServerWithTimeout(testingInstance, openAIServer, 4)
	requestURL := applicationServer.URL + "?prompt=ping&key=" + serviceSecretValue
	httpResponse, requestError := http.Get(requestURL)
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusGatewayTimeout {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		testingInstance.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBody))
	}
	if responsesAPICallCount < minimumExpectedCalls {
		testingInstance.Fatalf("calls=%d want>=%d", responsesAPICallCount, minimumExpectedCalls)
	}
}
