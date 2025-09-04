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

// TestIntegration_ModelSpec_SuppressesTemperatureAndTools_ForMini verifies that certain fields are suppressed for mini models.
func TestIntegration_ModelSpec_SuppressesTemperatureAndTools_ForMini(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)

	client, captured := makeHTTPClient(testingInstance, true)
	proxy.HTTPClient = client
	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     8,
	}, newLogger(testingInstance))
	if err != nil {
		testingInstance.Fatalf("BuildRouter failed: %v", err)
	}

	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set("prompt", "ping")
	queryValues.Set("key", "sekret")
	queryValues.Set("web_search", "1")
	queryValues.Set("model", "gpt-5-mini")
	requestURL.RawQuery = queryValues.Encode()

	httpResponse, requestError := http.Get(requestURL.String())
	if requestError != nil {
		testingInstance.Fatalf("GET failed: %v", requestError)
	}
	defer httpResponse.Body.Close()
	_, _ = io.ReadAll(httpResponse.Body)

	payload := *captured
	if _, ok := payload["temperature"]; ok {
		testingInstance.Fatalf("temperature must be omitted for gpt-5-mini, got: %v", payload["temperature"])
	}
	if _, ok := payload["tools"]; ok {
		testingInstance.Fatalf("tools must be omitted for gpt-5-mini, got: %v", payload["tools"])
	}
	if _, hasInput := payload["input"]; !hasInput {
		testingInstance.Fatalf("input must be present for responses API")
	}
	if _, hasMessages := payload["messages"]; hasMessages {
		testingInstance.Fatalf("messages must not be present for responses API payload")
	}
	time.Sleep(10 * time.Millisecond)
}
