package integration_test

import (
	"io"
	"net/http"
	"testing"
)

// TestProxyResponseDelivery verifies responses with and without web search.
func TestProxyResponseDelivery(testingInstance *testing.T) {
	testCases := []struct {
		name       string
		webSearch  bool
		body       string
		checkTools bool
	}{
		{name: "plain", webSearch: false, body: integrationOKBody},
		{name: "web_search", webSearch: true, body: integrationSearchBody, checkTools: true},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			var captured any
			var captureTarget *any
			if testCase.checkTools {
				captureTarget = &captured
			}
			openAIServer := newOpenAIServer(subTest, testCase.body, captureTarget)
			subTest.Cleanup(openAIServer.Close)
			applicationServer := newIntegrationServer(subTest, openAIServer)
			requestURL := applicationServer.URL + "?prompt=ping&key=" + serviceSecretValue
			if testCase.webSearch {
				requestURL += "&web_search=1"
			}
			httpResponse, requestError := http.Get(requestURL)
			if requestError != nil {
				subTest.Fatalf("request error: %v", requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != http.StatusOK {
				responseBody, _ := io.ReadAll(httpResponse.Body)
				subTest.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBody))
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if string(responseBytes) != testCase.body {
				subTest.Fatalf("body=%q want=%q", string(responseBytes), testCase.body)
			}
			if testCase.checkTools {
				capturedMap, _ := captured.(map[string]any)
				tools, ok := capturedMap["tools"].([]any)
				if !ok || len(tools) == 0 {
					subTest.Fatalf("tools missing when web_search=1")
				}
				first, _ := tools[0].(map[string]any)
				if first["type"] != "web_search" {
					subTest.Fatalf("tool type=%v want=web_search", first["type"])
				}
			}
		})
	}
}
