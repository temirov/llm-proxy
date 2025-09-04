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

func TestIntegration_WebSearch_UnsupportedModel_Returns400(testingContext *testing.T) {
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

	httpRequest, _ := http.NewRequest("GET", applicationServer.URL+"/?prompt=x&key=sekret&model=gpt-4o-mini&web_search=1", nil)
	httpResponse, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		testingContext.Fatalf("request failed: %v", err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusBadRequest {
		testingContext.Fatalf("status=%d want=%d", httpResponse.StatusCode, http.StatusBadRequest)
	}
	responseBody, _ := io.ReadAll(httpResponse.Body)
	if !strings.Contains(string(responseBody), "web_search is not supported") {
		testingContext.Fatalf("body=%q missing capability message", string(responseBody))
	}
}

func TestIntegration_TemperatureUnsupportedModel_RetriesWithoutTemperature(testingContext *testing.T) {
	var observed any

	openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch {
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/models"):
			io.WriteString(responseWriter, `{"data":[{"id":"gpt-5-mini"}]}`)
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/responses"):
			body, _ := io.ReadAll(httpRequest.Body)
			_ = json.Unmarshal(body, &observed)
			if strings.Contains(string(body), `"temperature"`) {
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

	httpResponse, err := http.Get(applicationServer.URL + "/?prompt=hello&key=sekret&model=gpt-5-mini")
	if err != nil {
		testingContext.Fatalf("request failed: %v", err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		responseBodyBytes, _ := io.ReadAll(httpResponse.Body)
		testingContext.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBodyBytes))
	}
	responseBodyBytes, _ := io.ReadAll(httpResponse.Body)
	if strings.TrimSpace(string(responseBodyBytes)) != "TEMPLESS_OK" {
		testingContext.Fatalf("body=%q want %q", string(responseBodyBytes), "TEMPLESS_OK")
	}
}
