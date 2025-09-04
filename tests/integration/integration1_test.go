package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

// newIntegrationServerWithTimeout builds the application server pointing at the stub OpenAI server with a configurable request timeout.
func newIntegrationServerWithTimeout(testingInstance *testing.T, openAIServer *httptest.Server, requestTimeoutSeconds int) *httptest.Server {
	testingInstance.Helper()
	proxy.SetModelsURL(openAIServer.URL + integrationModelsPath)
	proxy.SetResponsesURL(openAIServer.URL + integrationResponsesPath)
	proxy.HTTPClient = openAIServer.Client()
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)
	logger, _ := zap.NewDevelopment()
	testingInstance.Cleanup(func() { _ = logger.Sync() })
	router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:         integrationServiceSecret,
		OpenAIKey:             integrationOpenAIKey,
		LogLevel:              logLevelDebug,
		WorkerCount:           1,
		QueueSize:             4,
		RequestTimeoutSeconds: requestTimeoutSeconds,
	}, logger.Sugar())
	if buildRouterError != nil {
		testingInstance.Fatalf(buildRouterErrorFormat, buildRouterError)
	}
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)
	return server
}

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
			requestURL := applicationServer.URL + "?prompt=ping&key=" + integrationServiceSecret
			if testCase.webSearch {
				requestURL += "&web_search=1"
			}
			httpResponse, requestError := http.Get(requestURL)
			if requestError != nil {
				subTest.Fatalf(requestErrorFormat, requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != http.StatusOK {
				responseBody, _ := io.ReadAll(httpResponse.Body)
				subTest.Fatalf(unexpectedStatusFormat, httpResponse.StatusCode, string(responseBody))
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if string(responseBytes) != testCase.body {
				subTest.Fatalf(bodyMismatchFormat, string(responseBytes), testCase.body)
			}
			if testCase.checkTools {
				capturedMap, _ := captured.(map[string]any)
				tools, ok := capturedMap["tools"].([]any)
				if !ok || len(tools) == 0 {
					subTest.Fatalf(toolsMissingMessage)
				}
				first, _ := tools[0].(map[string]any)
				if first["type"] != "web_search" {
					subTest.Fatalf(toolTypeMismatchFormat, first["type"])
				}
			}
		})
	}
}
