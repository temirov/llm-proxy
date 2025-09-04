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
	testCases := []struct{ name string }{{name: "gpt_5_mini"}}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			client, captured := makeHTTPClient(subTest, true)
			configureProxy(subTest, client)
			router, err := proxy.BuildRouter(proxy.Configuration{
				ServiceSecret: serviceSecretValue,
				OpenAIKey:     openAIKeyValue,
				LogLevel:      "debug",
				WorkerCount:   1,
				QueueSize:     8,
			}, newLogger(subTest))
			if err != nil {
				subTest.Fatalf("BuildRouter failed: %v", err)
			}
			server := httptest.NewServer(router)
			subTest.Cleanup(server.Close)
			requestURL, _ := url.Parse(server.URL)
			queryValues := requestURL.Query()
			queryValues.Set(promptQueryParameter, promptValue)
			queryValues.Set(keyQueryParameter, serviceSecretValue)
			queryValues.Set(webSearchQueryParameter, "1")
			queryValues.Set(adaptiveModelQueryParameter, "gpt-5-mini")
			requestURL.RawQuery = queryValues.Encode()
			httpResponse, requestError := http.Get(requestURL.String())
			if requestError != nil {
				subTest.Fatalf("GET failed: %v", requestError)
			}
			defer httpResponse.Body.Close()
			_, _ = io.ReadAll(httpResponse.Body)
			payload := *captured
			if _, ok := payload["temperature"]; ok {
				subTest.Fatalf("temperature must be omitted for gpt-5-mini, got: %v", payload["temperature"])
			}
			if _, ok := payload["tools"]; ok {
				subTest.Fatalf("tools must be omitted for gpt-5-mini, got: %v", payload["tools"])
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
