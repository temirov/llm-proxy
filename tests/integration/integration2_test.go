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
	"github.com/temirov/llm-proxy/internal/constants"
	"github.com/temirov/llm-proxy/internal/proxy"
)

const webSearchQueryParameter = "web_search"

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
			endpointConfiguration := configureProxy(subTest, client)
			router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
				ServiceSecret: serviceSecretValue,
				OpenAIKey:     openAIKeyValue,
				LogLevel:      logLevelDebug,
				WorkerCount:   1,
				QueueSize:     8,
				Endpoints:     endpointConfiguration,
			}, newLogger(subTest))
			if buildRouterError != nil {
				subTest.Fatalf(buildRouterFailedFormat, buildRouterError)
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
				subTest.Fatalf(getFailedFormat, requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != http.StatusOK {
				subTest.Fatalf(statusWantFormat, httpResponse.StatusCode, http.StatusOK)
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if string(responseBytes) != testCase.expected {
				subTest.Fatalf(bodyMismatchFormat, string(responseBytes), testCase.expected)
			}
			if testCase.checkTools {
				tools, ok := (*captured)["tools"].([]any)
				if !ok || len(tools) == 0 {
					subTest.Fatalf(toolsMissingFormat, *captured)
				}
				first, _ := tools[0].(map[string]any)
				if first["type"] != "web_search" {
					subTest.Fatalf(toolTypeMismatchFormat, first["type"])
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
			config:      proxy.Configuration{ServiceSecret: constants.EmptyString, OpenAIKey: openAIKeyValue},
			expectError: "SERVICE_SECRET",
		},
		{
			name:        "missing_openai_key",
			config:      proxy.Configuration{ServiceSecret: serviceSecretValue, OpenAIKey: constants.EmptyString},
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
			if testCase.expectError != constants.EmptyString {
				_, buildRouterError := proxy.BuildRouter(testCase.config, newLogger(subTest))
				if buildRouterError == nil || !strings.Contains(buildRouterError.Error(), testCase.expectError) {
					subTest.Fatalf(expectedErrorFormat, testCase.expectError, buildRouterError)
				}
				return
			}
			client, _ := makeHTTPClient(subTest, false)
			endpointConfiguration := configureProxy(subTest, client)
			testCase.config.Endpoints = endpointConfiguration
			router, buildRouterError := proxy.BuildRouter(testCase.config, newLogger(subTest))
			if buildRouterError != nil {
				subTest.Fatalf(buildRouterFailedFormat, buildRouterError)
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
				subTest.Fatalf(getFailedFormat, requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != testCase.expectedStatus {
				var bodyBuffer bytes.Buffer
				_, _ = io.Copy(&bodyBuffer, httpResponse.Body)
				subTest.Fatalf(statusWantBodyFormat, httpResponse.StatusCode, testCase.expectedStatus, bodyBuffer.String())
			}
		})
	}
}
