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

func TestIntegration_ModelSpec_SuppressesTemperatureAndTools_ForMini(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client, captured := makeHTTPClient(t, true)
	proxy.HTTPClient = client
	proxy.ModelsURL = "https://mock.local/v1/models"
	proxy.ResponsesURL = "https://mock.local/v1/responses"

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     8,
	}, newLogger(t))
	if err != nil {
		t.Fatalf("BuildRouter failed: %v", err)
	}

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	q := u.Query()
	q.Set("prompt", "ping")
	q.Set("key", "sekret")
	q.Set("web_search", "1")
	q.Set("model", "gpt-5-mini")
	u.RawQuery = q.Encode()

	res, err := http.Get(u.String())
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer res.Body.Close()
	_, _ = io.ReadAll(res.Body)

	payload := *captured
	if _, ok := payload["temperature"]; ok {
		t.Fatalf("temperature must be omitted for gpt-5-mini, got: %v", payload["temperature"])
	}
	if _, ok := payload["tools"]; ok {
		t.Fatalf("tools must be omitted for gpt-5-mini, got: %v", payload["tools"])
	}
	if _, hasInput := payload["input"]; !hasInput {
		t.Fatalf("input must be present for responses API")
	}
	if _, hasMessages := payload["messages"]; hasMessages {
		t.Fatalf("messages must not be present for responses API payload")
	}
	time.Sleep(10 * time.Millisecond)
}
