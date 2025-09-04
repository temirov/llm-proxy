package tests_test

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

// TestIntegration_WebSearch_UnsupportedModel_Returns400 verifies that a request
// with web search enabled for an unsupported model results in a bad request.
func TestIntegration_WebSearch_UnsupportedModel_Returns400(t *testing.T) {
	openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch {
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/models"):
			io.WriteString(responseWriter, `{"data":[{"id":"gpt-4o-mini"},{"id":"gpt-4o"}]}`)
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/responses"):
			io.WriteString(responseWriter, `{"output_text":"SHOULD_NOT_BE_CALLED"}`)
		default:
			http.NotFound(responseWriter, httpRequest)
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

	proxyRequest, _ := http.NewRequest("GET", proxyServer.URL+"/?prompt=x&key=sekret&model=gpt-4o-mini&web_search=1", nil)
	proxyResponse, err := http.DefaultClient.Do(proxyRequest)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer proxyResponse.Body.Close()

	if proxyResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d", proxyResponse.StatusCode, http.StatusBadRequest)
	}
	responseBody, _ := io.ReadAll(proxyResponse.Body)
	if !strings.Contains(string(responseBody), "web_search is not supported") {
		t.Fatalf("body=%q missing capability message", string(responseBody))
	}
}

// TestIntegration_TemperatureUnsupportedModel_RetriesWithoutTemperature ensures
// that the proxy retries requests without the temperature parameter when the
// upstream model does not support it.
func TestIntegration_TemperatureUnsupportedModel_RetriesWithoutTemperature(t *testing.T) {
	var observed any

	openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch {
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/models"):
			io.WriteString(responseWriter, `{"data":[{"id":"gpt-5-mini"}]}`)
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/responses"):
			requestBody, _ := io.ReadAll(httpRequest.Body)
			_ = json.Unmarshal(requestBody, &observed)
			if strings.Contains(string(requestBody), `"temperature"`) {
				responseWriter.WriteHeader(http.StatusBadRequest)
				io.WriteString(responseWriter, `{"error":{"message":"Unsupported parameter: 'temperature'"}}`)
				return
			}
			io.WriteString(responseWriter, `{"output_text":"TEMPLESS_OK"}`)
		default:
			http.NotFound(responseWriter, httpRequest)
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

	proxyResponse, err := http.Get(proxyServer.URL + "/?prompt=hello&key=sekret&model=gpt-5-mini")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer proxyResponse.Body.Close()

	if proxyResponse.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(proxyResponse.Body)
		t.Fatalf("status=%d body=%s", proxyResponse.StatusCode, string(bodyBytes))
	}
	bodyBytes, _ := io.ReadAll(proxyResponse.Body)
	if strings.TrimSpace(string(bodyBytes)) != "TEMPLESS_OK" {
		t.Fatalf("body=%q want %q", string(bodyBytes), "TEMPLESS_OK")
	}
}
