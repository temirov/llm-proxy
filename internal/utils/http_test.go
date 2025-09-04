package utils_test

import (
	"bytes"
	"testing"

	"github.com/temirov/llm-proxy/internal/utils"
)

const (
	httpMethodGet      = "GET"
	requestURLExample  = "http://example.com"
	headerNameExample  = "X-Test-Header"
	headerValueExample = "header-value"
	invalidRequestURL  = "://bad-url"
	bodyContent        = "body"
)

type buildHTTPRequestTestDefinition struct {
	testName            string
	method              string
	requestURL          string
	headers             map[string]string
	expectError         bool
	expectedHeaderValue string
}

// TestBuildHTTPRequestWithHeaders_ConstructsRequests verifies that BuildHTTPRequestWithHeaders creates requests and applies headers.
func TestBuildHTTPRequestWithHeaders_ConstructsRequests(testingInstance *testing.T) {
	testCases := []buildHTTPRequestTestDefinition{
		{
			testName:            "valid request",
			method:              httpMethodGet,
			requestURL:          requestURLExample,
			headers:             map[string]string{headerNameExample: headerValueExample},
			expectError:         false,
			expectedHeaderValue: headerValueExample,
		},
		{
			testName:    "invalid url",
			method:      httpMethodGet,
			requestURL:  invalidRequestURL,
			headers:     map[string]string{},
			expectError: true,
		},
	}
	for _, currentTestCase := range testCases {
		testingInstance.Run(currentTestCase.testName, func(nestedTestingInstance *testing.T) {
			httpRequest, buildRequestError := utils.BuildHTTPRequestWithHeaders(currentTestCase.method, currentTestCase.requestURL, bytes.NewBufferString(bodyContent), currentTestCase.headers)
			if currentTestCase.expectError {
				if buildRequestError == nil {
					nestedTestingInstance.Fatalf("expected error but got none")
				}
				return
			}
			if buildRequestError != nil {
				nestedTestingInstance.Fatalf("unexpected error: %v", buildRequestError)
			}
			headerValue := httpRequest.Header.Get(headerNameExample)
			if headerValue != currentTestCase.expectedHeaderValue {
				nestedTestingInstance.Fatalf("header value=%s expected=%s", headerValue, currentTestCase.expectedHeaderValue)
			}
		})
	}
}
