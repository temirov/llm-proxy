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

// TestIntegration_ResponseDelivered verifies that the integration endpoint returns the upstream response text.
func TestIntegration_ResponseDelivered(testingInstance *testing.T) {
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

	proxy.SetModelsURL(openAIServer.URL + "/v1/models")
	proxy.SetResponsesURL(openAIServer.URL + "/v1/responses")
	proxy.HTTPClient = openAIServer.Client()
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)

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
		testingInstance.Fatalf("BuildRouter error: %v", err)
	}

	applicationServer := httptest.NewServer(router)
	defer applicationServer.Close()

	httpResponse, requestError := http.Get(applicationServer.URL + "/?prompt=ping&key=sekret")
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		testingInstance.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBody))
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if got := strings.TrimSpace(string(responseBytes)); got != "INTEGRATION_OK" {
		testingInstance.Fatalf("body=%q; want INTEGRATION_OK", got)
	}
}

// TestIntegration_ResponseDelivered_WithWebSearch verifies delivery of responses when web search is requested.
func TestIntegration_ResponseDelivered_WithWebSearch(testingInstance *testing.T) {
	var captured any

	openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
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
	defer openAIServer.Close()

	proxy.SetModelsURL(openAIServer.URL + "/v1/models")
	proxy.SetResponsesURL(openAIServer.URL + "/v1/responses")
	proxy.HTTPClient = openAIServer.Client()
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)

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
		testingInstance.Fatalf("BuildRouter error: %v", err)
	}

	applicationServer := httptest.NewServer(router)
	defer applicationServer.Close()

	httpResponse, requestError := http.Get(applicationServer.URL + "/?prompt=ping&key=sekret&web_search=1")
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		testingInstance.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBody))
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if got := strings.TrimSpace(string(responseBytes)); got != "SEARCH_OK" {
		testingInstance.Fatalf("body=%q; want SEARCH_OK", got)
	}

	// Assert that the tool was sent.
	m, _ := captured.(map[string]any)
	tools, ok := m["tools"].([]any)
	if !ok || len(tools) == 0 {
		testingInstance.Fatalf("tools missing when web_search=1")
	}
	first, _ := tools[0].(map[string]any)
	if first["type"] != "web_search" {
		testingInstance.Fatalf("tool type=%v; want web_search", first["type"])
	}
}
