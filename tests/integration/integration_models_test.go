package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/proxy"
)

// TestIntegrationModelSpecSuppression verifies that certain fields are suppressed for mini models.
func TestIntegrationModelSpecSuppression(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	testCases := []struct {
		name  string
		model string
	}{{name: "gpt_5_mini", model: proxy.ModelNameGPT5Mini}}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			client, captured := makeHTTPClient(subTest, true)
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
			queryValues.Set(webSearchQueryParameter, "1")
			queryValues.Set(adaptiveModelQueryParameter, testCase.model)
			requestURL.RawQuery = queryValues.Encode()
			httpResponse, requestError := http.Get(requestURL.String())
			if requestError != nil {
				subTest.Fatalf("GET failed: %v", requestError)
			}
			defer httpResponse.Body.Close()
			_, _ = io.ReadAll(httpResponse.Body)
			payload := *captured
			if _, ok := payload["temperature"]; ok {
				subTest.Fatalf("temperature must be omitted for %s, got: %v", testCase.model, payload["temperature"])
			}
			if _, ok := payload["tools"]; ok {
				subTest.Fatalf("tools must be omitted for %s, got: %v", testCase.model, payload["tools"])
			}
			if _, hasInput := payload["input"]; !hasInput {
				subTest.Fatalf("input must be present for responses API")
			}
			if _, hasMessages := payload["messages"]; hasMessages {
				subTest.Fatalf("messages must not be present for responses API payload")
			}
			time.Sleep(10 * time.Millisecond)
		})
	}
}
