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
	// testCaseWithoutKeyName identifies the scenario lacking the key query parameter.
	testCaseWithoutKeyName = "without_key_query_parameter"
	// testCaseWithKeyName identifies the scenario including the key query parameter.
	testCaseWithKeyName = "with_key_query_parameter"
)

// missingKeyTestCase defines the inputs and expectations for key parameter handling scenarios.
type missingKeyTestCase struct {
	name           string
	includeKey     bool
	expectedStatus int
	expectedBody   string
}

// TestClientKeyHandling verifies responses for requests with and without the key query parameter.
func TestClientKeyHandling(testingInstance *testing.T) {
	testCases := []missingKeyTestCase{
		{
			name:           testCaseWithoutKeyName,
			includeKey:     false,
			expectedStatus: http.StatusForbidden,
			expectedBody:   missingKeyErrorBody,
		},
		{
			name:           testCaseWithKeyName,
			includeKey:     true,
			expectedStatus: http.StatusOK,
			expectedBody:   integrationOKBody,
		},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			openAIServer := newOpenAIServer(subTest, integrationOKBody, nil)
			subTest.Cleanup(openAIServer.Close)
			applicationServer := newIntegrationServer(subTest, openAIServer)
			requestURL, _ := url.Parse(applicationServer.URL)
			queryValues := requestURL.Query()
			queryValues.Set(promptQueryParameter, promptValue)
			if testCase.includeKey {
				queryValues.Set(keyQueryParameter, integrationServiceSecret)
			}
			requestURL.RawQuery = queryValues.Encode()
			httpResponse, requestError := http.Get(requestURL.String())
			if requestError != nil {
				subTest.Fatalf(requestErrorFormat, requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != testCase.expectedStatus {
				responseBody, _ := io.ReadAll(httpResponse.Body)
				subTest.Fatalf(unexpectedStatusFormat, httpResponse.StatusCode, string(responseBody))
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if string(responseBytes) != testCase.expectedBody {
				subTest.Fatalf(bodyMismatchFormat, string(responseBytes), testCase.expectedBody)
			}
		})
	}
}
