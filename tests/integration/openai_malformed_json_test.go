package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const (
	// malformedJSONPayload is the body returned by the stubbed upstream to simulate invalid JSON.
	malformedJSONPayload = "invalid"
	// expectedErrorMessage is the error returned by the proxy when upstream JSON cannot be parsed.
	expectedErrorMessage = "OpenAI API error"
	// contentTypeJSON is the HTTP Content-Type header value for JSON payloads.
	contentTypeJSON = "application/json"
)

// newMalformedOpenAIServer returns a stub OpenAI server emitting invalid JSON for the responses endpoint.
func newMalformedOpenAIServer(testingInstance *testing.T) *httptest.Server {
	testingInstance.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch httpRequest.URL.Path {
		case integrationModelsPath:
			responseWriter.Header().Set("Content-Type", contentTypeJSON)
			_, _ = io.WriteString(responseWriter, integrationModelListBody)
		case integrationResponsesPath:
			responseWriter.Header().Set("Content-Type", contentTypeJSON)
			_, _ = io.WriteString(responseWriter, malformedJSONPayload)
		default:
			http.NotFound(responseWriter, httpRequest)
		}
	}))
	return server
}

// TestOpenAIMalformedJSON verifies that the proxy returns a 502 error when the upstream responds with invalid JSON.
func TestOpenAIMalformedJSON(testingInstance *testing.T) {
	openAIServer := newMalformedOpenAIServer(testingInstance)
	testingInstance.Cleanup(openAIServer.Close)
	applicationServer := newIntegrationServer(testingInstance, openAIServer)
	requestURL, _ := url.Parse(applicationServer.URL)
	queryValues := requestURL.Query()
	queryValues.Set(promptQueryParameter, promptValue)
	queryValues.Set(keyQueryParameter, integrationServiceSecret)
	requestURL.RawQuery = queryValues.Encode()
	httpResponse, requestError := http.Get(requestURL.String())
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusBadGateway {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		testingInstance.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBody))
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if string(responseBytes) != expectedErrorMessage {
		testingInstance.Fatalf("body=%q want=%q", string(responseBytes), expectedErrorMessage)
	}
}
