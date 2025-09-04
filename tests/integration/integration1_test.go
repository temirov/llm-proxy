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

func TestIntegration_ResponseDelivered(testingContext *testing.T) {
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
	testingContext.Cleanup(proxy.ResetModelsURL)
	testingContext.Cleanup(proxy.ResetResponsesURL)

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
		testingContext.Fatalf("BuildRouter error: %v", err)
	}

	applicationServer := httptest.NewServer(router)
	defer applicationServer.Close()

	httpResponse, err := http.Get(applicationServer.URL + "/?prompt=ping&key=sekret")
	if err != nil {
		testingContext.Fatalf("request error: %v", err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		responseBytes, _ := io.ReadAll(httpResponse.Body)
		testingContext.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBytes))
	}
	responseBody, _ := io.ReadAll(httpResponse.Body)
	if responseText := strings.TrimSpace(string(responseBody)); responseText != "INTEGRATION_OK" {
		testingContext.Fatalf("body=%q; want INTEGRATION_OK", responseText)
	}
}

func TestIntegration_ResponseDelivered_WithWebSearch(testingContext *testing.T) {
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
	testingContext.Cleanup(proxy.ResetModelsURL)
	testingContext.Cleanup(proxy.ResetResponsesURL)

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
		testingContext.Fatalf("BuildRouter error: %v", err)
	}

	applicationServer := httptest.NewServer(router)
	defer applicationServer.Close()

	httpResponse, err := http.Get(applicationServer.URL + "/?prompt=ping&key=sekret&web_search=1")
	if err != nil {
		testingContext.Fatalf("request error: %v", err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		responseBytes, _ := io.ReadAll(httpResponse.Body)
		testingContext.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBytes))
	}
	responseBody, _ := io.ReadAll(httpResponse.Body)
	if responseText := strings.TrimSpace(string(responseBody)); responseText != "SEARCH_OK" {
		testingContext.Fatalf("body=%q; want SEARCH_OK", responseText)
	}

	// Assert that the tool was sent.
	capturedMap, _ := captured.(map[string]any)
	tools, toolsFound := capturedMap["tools"].([]any)
	if !toolsFound || len(tools) == 0 {
		testingContext.Fatalf("tools missing when web_search=1")
	}
	firstTool, _ := tools[0].(map[string]any)
	if firstTool["type"] != "web_search" {
		testingContext.Fatalf("tool type=%v; want web_search", firstTool["type"])
	}
}
