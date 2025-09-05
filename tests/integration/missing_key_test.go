package integration_test

import (
	"io"
	"net/http"
	"net/url"
	"testing"
)

const (
	// missingKeyErrorBody is the expected response when the key query parameter is absent.
	missingKeyErrorBody = "unknown client key"
)

// TestMissingClientKeyReturnsForbidden verifies that a request without a key is rejected.
func TestMissingClientKeyReturnsForbidden(testingInstance *testing.T) {
	openAIServer := newOpenAIServer(testingInstance, integrationOKBody, nil)
	testingInstance.Cleanup(openAIServer.Close)
	applicationServer := newIntegrationServer(testingInstance, openAIServer)
	requestURL, _ := url.Parse(applicationServer.URL)
	queryValues := requestURL.Query()
	queryValues.Set(promptQueryParameter, promptValue)
	requestURL.RawQuery = queryValues.Encode()
	httpResponse, requestError := http.Get(requestURL.String())
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusForbidden {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		testingInstance.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBody))
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if string(responseBytes) != missingKeyErrorBody {
		testingInstance.Fatalf("body=%q want=%q", string(responseBytes), missingKeyErrorBody)
	}
}
