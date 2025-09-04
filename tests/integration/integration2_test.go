package integration_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/proxy"
)

const (
	webSearchQueryParameter = "web_search"
)

// TestClientResponseDelivery validates responses with and without web search.
func TestClientResponseDelivery(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	testCases := []struct {
		name       string
		webSearch  bool
		expected   string
		checkTools bool
	}{
		{name: "plain", webSearch: false, expected: integrationOKBody},
		{name: "web_search", webSearch: true, expected: integrationSearchBody, checkTools: true},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			client, captured := makeHTTPClient(subTest, testCase.webSearch)
			configureProxy(subTest, client)
			router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
				ServiceSecret: serviceSecretValue,
				OpenAIKey:     openAIKeyValue,
				LogLevel:      logLevelDebug,
				WorkerCount:   1,
				QueueSize:     8,
			}, newLogger(subTest))
			if buildRouterError != nil {
				subTest.Fatalf("BuildRouter failed: %v", buildRouterError)
			}
			server := httptest.NewServer(router)
			subTest.Cleanup(server.Close)
			requestURL, _ := url.Parse(server.URL)
			queryValues := requestURL.Query()
			queryValues.Set(promptQueryParameter, promptValue)
			queryValues.Set(keyQueryParameter, serviceSecretValue)
			if testCase.webSearch {
				queryValues.Set(webSearchQueryParameter, "1")
			}
			requestURL.RawQuery = queryValues.Encode()
			httpResponse, requestError := http.Get(requestURL.String())
			if requestError != nil {
				subTest.Fatalf("GET failed: %v", requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != http.StatusOK {
				subTest.Fatalf("status=%d want=%d", httpResponse.StatusCode, http.StatusOK)
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if string(responseBytes) != testCase.expected {
				subTest.Fatalf("body=%q want=%q", string(responseBytes), testCase.expected)
			}
			if testCase.checkTools {
				tools, ok := (*captured)["tools"].([]any)
				if !ok || len(tools) == 0 {
					subTest.Fatalf("tools missing in payload when web_search=1; captured=%v", *captured)
				}
				first, _ := tools[0].(map[string]any)
				if first["type"] != "web_search" {
					subTest.Fatalf("tool type=%v want=web_search", first["type"])
				}
			}
		})
	}
}

// TestIntegrationConfiguration covers configuration errors and wrong API keys.
func TestIntegrationConfiguration(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	testCases := []struct {
		name           string
		config         proxy.Configuration
		requestKey     string
		expectedStatus int
		expectError    string
	}{
		{
			name:        "missing_service_secret",
			config:      proxy.Configuration{ServiceSecret: "", OpenAIKey: openAIKeyValue},
			expectError: "SERVICE_SECRET",
		},
		{
			name:        "missing_openai_key",
			config:      proxy.Configuration{ServiceSecret: serviceSecretValue, OpenAIKey: ""},
			expectError: "OPENAI_API_KEY",
		},
		{
			name:           "wrong_key",
			config:         proxy.Configuration{ServiceSecret: serviceSecretValue, OpenAIKey: openAIKeyValue, LogLevel: logLevelDebug, WorkerCount: 1, QueueSize: 4},
			requestKey:     "wrong",
			expectedStatus: http.StatusForbidden,
		},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			if testCase.expectError != "" {
				_, buildRouterError := proxy.BuildRouter(testCase.config, newLogger(subTest))
				if buildRouterError == nil || !strings.Contains(buildRouterError.Error(), testCase.expectError) {
					subTest.Fatalf("expected %s error, got %v", testCase.expectError, buildRouterError)
				}
				return
			}
			client, _ := makeHTTPClient(subTest, false)
			configureProxy(subTest, client)
			router, buildRouterError := proxy.BuildRouter(testCase.config, newLogger(subTest))
			if buildRouterError != nil {
				subTest.Fatalf("BuildRouter failed: %v", buildRouterError)
			}
			server := httptest.NewServer(router)
			subTest.Cleanup(server.Close)
			requestURL, _ := url.Parse(server.URL)
			queryValues := requestURL.Query()
			queryValues.Set(promptQueryParameter, promptValue)
			queryValues.Set(keyQueryParameter, testCase.requestKey)
			requestURL.RawQuery = queryValues.Encode()
			httpResponse, requestError := http.Get(requestURL.String())
			if requestError != nil {
				subTest.Fatalf("GET failed: %v", requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != testCase.expectedStatus {
				var buf bytes.Buffer
				_, _ = io.Copy(&buf, httpResponse.Body)
				subTest.Fatalf("status=%d want=%d body=%q", httpResponse.StatusCode, testCase.expectedStatus, buf.String())
			}
		})
	}
}
