package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

const (
	availableModelsBody     = `{"data":[{"id":"gpt-4.1"},{"id":"gpt-5-mini"}]}`
	webSearchQueryParameter = "web_search"
)

// roundTripperFunc stubs both models and responses endpoints.
type roundTripperFunc func(httpRequest *http.Request) (*http.Response, error)

func (roundTripper roundTripperFunc) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	return roundTripper(httpRequest)
}

// makeHTTPClient returns a stub HTTP client capturing payloads and returning canned responses.
func makeHTTPClient(testingInstance *testing.T, wantWebSearch bool) (*http.Client, *map[string]any) {
	testingInstance.Helper()
	var captured map[string]any
	return &http.Client{
		Transport: roundTripperFunc(func(httpRequest *http.Request) (*http.Response, error) {
			switch httpRequest.URL.String() {
			case proxy.ModelsURL():
				body := availableModelsBody
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			case proxy.ResponsesURL():
				if httpRequest.Body != nil {
					buf, _ := io.ReadAll(httpRequest.Body)
					_ = json.Unmarshal(buf, &captured)
				}
				text := integrationOKBody
				if wantWebSearch {
					text = integrationSearchBody
				}
				body := `{"output_text":"` + text + `"}`
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			default:
				testingInstance.Fatalf("unexpected request to %s", httpRequest.URL.String())
				return nil, nil
			}
		}),
		Timeout: 5 * time.Second,
	}, &captured
}

// newLogger constructs a development logger for tests.
func newLogger(testingInstance *testing.T) *zap.SugaredLogger {
	testingInstance.Helper()
	loggerInstance, _ := zap.NewDevelopment()
	testingInstance.Cleanup(func() { _ = loggerInstance.Sync() })
	return loggerInstance.Sugar()
}

// configureProxy sets URLs and the HTTP client for proxy operations.
func configureProxy(testingInstance *testing.T, client *http.Client) {
	testingInstance.Helper()
	proxy.HTTPClient = client
	proxy.SetModelsURL(mockModelsURL)
	proxy.SetResponsesURL(mockResponsesURL)
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)
}

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
				var bodyBuffer bytes.Buffer
				_, _ = io.Copy(&bodyBuffer, httpResponse.Body)
				subTest.Fatalf("status=%d want=%d body=%q", httpResponse.StatusCode, testCase.expectedStatus, bodyBuffer.String())
			}
		})
	}
}
