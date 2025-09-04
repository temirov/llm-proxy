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

func TestIntegration_ModelSpec_SuppressesTemperatureAndTools_ForMini(testingContext *testing.T) {
	gin.SetMode(gin.TestMode)

	client, captured := makeHTTPClient(testingContext, true)
	proxy.HTTPClient = client
	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	testingContext.Cleanup(proxy.ResetModelsURL)
	testingContext.Cleanup(proxy.ResetResponsesURL)

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     8,
	}, newLogger(testingContext))
	if err != nil {
		testingContext.Fatalf("BuildRouter failed: %v", err)
	}

	testServer := httptest.NewServer(router)
	testingContext.Cleanup(testServer.Close)

	parsedURL, _ := url.Parse(testServer.URL)
	queryValues := parsedURL.Query()
	queryValues.Set("prompt", "ping")
	queryValues.Set("key", "sekret")
	queryValues.Set("web_search", "1")
	queryValues.Set("model", "gpt-5-mini")
	parsedURL.RawQuery = queryValues.Encode()

	httpResponse, err := http.Get(parsedURL.String())
	if err != nil {
		testingContext.Fatalf("GET failed: %v", err)
	}
	defer httpResponse.Body.Close()
	_, _ = io.ReadAll(httpResponse.Body)

	payload := *captured
	if _, valueFound := payload["temperature"]; valueFound {
		testingContext.Fatalf("temperature must be omitted for gpt-5-mini, got: %v", payload["temperature"])
	}
	if _, valueFound := payload["tools"]; valueFound {
		testingContext.Fatalf("tools must be omitted for gpt-5-mini, got: %v", payload["tools"])
	}
	if _, hasInput := payload["input"]; !hasInput {
		testingContext.Fatalf("input must be present for responses API")
	}
	if _, hasMessages := payload["messages"]; hasMessages {
		testingContext.Fatalf("messages must not be present for responses API payload")
	}
	time.Sleep(10 * time.Millisecond)
}
