package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

// TestIntegration_ResponseDelivered ensures that the proxy returns a response
// from the upstream provider without web search enabled.
func TestIntegration_ResponseDelivered(t *testing.T) {
	// Fake upstream that serves /v1/models and /v1/responses.
	openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch {
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/models"):
			responseWriter.Header().Set("Content-Type", "application/json")
			io.WriteString(responseWriter, `{"object":"list","data":[{"id":"gpt-4.1","object":"model"}]}`)
			return
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/responses"):
			responseWriter.Header().Set("Content-Type", "application/json")
			io.WriteString(responseWriter, `{"output_text":"INTEGRATION_OK"}`)
			return
		default:
			http.NotFound(responseWriter, httpRequest)
			return
		}
	}))
	defer openAIServer.Close()

	// Inject URLs + client.
	proxy.SetModelsURL(openAIServer.URL + "/v1/models")
	proxy.SetResponsesURL(openAIServer.URL + "/v1/responses")
	proxy.HTTPClient = openAIServer.Client()
	t.Cleanup(proxy.ResetModelsURL)
	t.Cleanup(proxy.ResetResponsesURL)

	// Build app router and serve it.
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     4,
	}, logger.Sugar())
	if err != nil {
		t.Fatalf("BuildRouter error: %v", err)
	}

	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	proxyResponse, err := http.Get(proxyServer.URL + "/?prompt=ping&key=sekret")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer proxyResponse.Body.Close()

	if proxyResponse.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(proxyResponse.Body)
		t.Fatalf("status=%d body=%s", proxyResponse.StatusCode, string(bodyBytes))
	}
	responseBody, _ := io.ReadAll(proxyResponse.Body)
	if got := strings.TrimSpace(string(responseBody)); got != "INTEGRATION_OK" {
		t.Fatalf("body=%q; want INTEGRATION_OK", got)
	}
}

// TestIntegration_ResponseDelivered_WithWebSearch ensures that web search is
// forwarded to the upstream provider when requested.
func TestIntegration_ResponseDelivered_WithWebSearch(t *testing.T) {
	var captured any

	openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch {
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/models"):
			responseWriter.Header().Set("Content-Type", "application/json")
			io.WriteString(responseWriter, `{"object":"list","data":[{"id":"gpt-4.1","object":"model"}]}`)
			return
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/responses"):
			requestBody, _ := io.ReadAll(httpRequest.Body)
			_ = json.Unmarshal(requestBody, &captured)
			responseWriter.Header().Set("Content-Type", "application/json")
			io.WriteString(responseWriter, `{"output_text":"SEARCH_OK"}`)
			return
		default:
			http.NotFound(responseWriter, httpRequest)
			return
		}
	}))
	defer openAIServer.Close()

	proxy.SetModelsURL(openAIServer.URL + "/v1/models")
	proxy.SetResponsesURL(openAIServer.URL + "/v1/responses")
	proxy.HTTPClient = openAIServer.Client()
	t.Cleanup(proxy.ResetModelsURL)
	t.Cleanup(proxy.ResetResponsesURL)

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     4,
	}, logger.Sugar())
	if err != nil {
		t.Fatalf("BuildRouter error: %v", err)
	}

	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	proxyResponse, err := http.Get(proxyServer.URL + "/?prompt=ping&key=sekret&web_search=1")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer proxyResponse.Body.Close()

	if proxyResponse.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(proxyResponse.Body)
		t.Fatalf("status=%d body=%s", proxyResponse.StatusCode, string(bodyBytes))
	}
	responseBody, _ := io.ReadAll(proxyResponse.Body)
	if got := strings.TrimSpace(string(responseBody)); got != "SEARCH_OK" {
		t.Fatalf("body=%q; want SEARCH_OK", got)
	}

	// Assert that the tool was sent.
	m, _ := captured.(map[string]any)
	tools, ok := m["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("tools missing when web_search=1")
	}
	first, _ := tools[0].(map[string]any)
	if first["type"] != "web_search" {
		t.Fatalf("tool type=%v; want web_search", first["type"])
	}
}
