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

func TestIntegration_ResponseDelivered(t *testing.T) {
	// Fake upstream that serves /v1/models and /v1/responses.
	openAISrv := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
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
	defer openAISrv.Close()

	// Inject URLs + client.
	proxy.SetModelsURL(openAISrv.URL + "/v1/models")
	proxy.SetResponsesURL(openAISrv.URL + "/v1/responses")
	proxy.HTTPClient = openAISrv.Client()
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

	appSrv := httptest.NewServer(router)
	defer appSrv.Close()

	resp, err := http.Get(appSrv.URL + "/?prompt=ping&key=sekret")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(b))
	}
	body, _ := io.ReadAll(resp.Body)
	if got := strings.TrimSpace(string(body)); got != "INTEGRATION_OK" {
		t.Fatalf("body=%q; want INTEGRATION_OK", got)
	}
}

func TestIntegration_ResponseDelivered_WithWebSearch(t *testing.T) {
	var captured any

	openAISrv := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch {
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/models"):
			responseWriter.Header().Set("Content-Type", "application/json")
			io.WriteString(responseWriter, `{"object":"list","data":[{"id":"gpt-4.1","object":"model"}]}`)
			return
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/responses"):
			body, _ := io.ReadAll(httpRequest.Body)
			_ = json.Unmarshal(body, &captured)
			responseWriter.Header().Set("Content-Type", "application/json")
			io.WriteString(responseWriter, `{"output_text":"SEARCH_OK"}`)
			return
		default:
			http.NotFound(responseWriter, httpRequest)
			return
		}
	}))
	defer openAISrv.Close()

	proxy.SetModelsURL(openAISrv.URL + "/v1/models")
	proxy.SetResponsesURL(openAISrv.URL + "/v1/responses")
	proxy.HTTPClient = openAISrv.Client()
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

	appSrv := httptest.NewServer(router)
	defer appSrv.Close()

	resp, err := http.Get(appSrv.URL + "/?prompt=ping&key=sekret&web_search=1")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(b))
	}
	body, _ := io.ReadAll(resp.Body)
	if got := strings.TrimSpace(string(body)); got != "SEARCH_OK" {
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
