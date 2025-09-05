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

// Test constants.
const (
	modelIDGPT4o    = "gpt-4o"
	modelIDGPT5Mini = "gpt-5-mini"
	serviceSecret   = "sekret"
	openAIKey       = "sk-test"
	logLevel        = "debug"
)

// TestIntegration_TemperatureNotAllowed_OmitsParameter confirms that temperature is omitted when not allowed by metadata.
func TestIntegration_TemperatureNotAllowed_OmitsParameter(testingInstance *testing.T) {
	var observed any

	openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch {
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/models"):
			io.WriteString(responseWriter, `{"data":[{"id":"`+modelIDGPT5Mini+`"}]}`)
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/models/"+modelIDGPT5Mini):
			io.WriteString(responseWriter, `{"allowed_request_fields":[]}`)
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/responses"):
			body, _ := io.ReadAll(httpRequest.Body)
			_ = json.Unmarshal(body, &observed)
			io.WriteString(responseWriter, `{"output_text":"TEMPLESS_OK"}`)
		default:
			http.NotFound(responseWriter, httpRequest)
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

	router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: serviceSecret,
		OpenAIKey:     openAIKey,
		LogLevel:      logLevel,
		WorkerCount:   1,
		QueueSize:     4,
	}, logger.Sugar())
	if buildRouterError != nil {
		testingInstance.Fatalf("BuildRouter error: %v", buildRouterError)
	}

	applicationServer := httptest.NewServer(router)
	defer applicationServer.Close()

	httpResponse, requestError := http.Get(applicationServer.URL + "/?prompt=hello&key=" + serviceSecret + "&model=" + modelIDGPT5Mini)
	if requestError != nil {
		testingInstance.Fatalf("request failed: %v", requestError)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		testingInstance.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBody))
	}
	if payload, ok := observed.(map[string]any); ok {
		if _, found := payload["temperature"]; found {
			testingInstance.Fatalf("temperature present in payload: %v", payload)
		}
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if strings.TrimSpace(string(responseBytes)) != "TEMPLESS_OK" {
		testingInstance.Fatalf("body=%q want %q", string(responseBytes), "TEMPLESS_OK")
	}
}

// TestIntegration_ToolsNotAllowed_OmitsParameters verifies that tool parameters are omitted when metadata disallows them.
func TestIntegration_ToolsNotAllowed_OmitsParameters(testingInstance *testing.T) {
	var observed any

	openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch {
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/models"):
			io.WriteString(responseWriter, `{"data":[{"id":"`+modelIDGPT4o+`"}]}`)
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/models/"+modelIDGPT4o):
			io.WriteString(responseWriter, `{"allowed_request_fields":["temperature"]}`)
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/responses"):
			body, _ := io.ReadAll(httpRequest.Body)
			_ = json.Unmarshal(body, &observed)
			io.WriteString(responseWriter, `{"output_text":"NO_TOOLS_OK"}`)
		default:
			http.NotFound(responseWriter, httpRequest)
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

	router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: serviceSecret,
		OpenAIKey:     openAIKey,
		LogLevel:      logLevel,
		WorkerCount:   1,
		QueueSize:     4,
	}, logger.Sugar())
	if buildRouterError != nil {
		testingInstance.Fatalf("BuildRouter error: %v", buildRouterError)
	}

	applicationServer := httptest.NewServer(router)
	defer applicationServer.Close()

	httpResponse, requestError := http.Get(applicationServer.URL + "/?prompt=hello&key=" + serviceSecret + "&model=" + modelIDGPT4o + "&web_search=1")
	if requestError != nil {
		testingInstance.Fatalf("request failed: %v", requestError)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		testingInstance.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBody))
	}
	if payload, ok := observed.(map[string]any); ok {
		if _, found := payload["tools"]; found {
			testingInstance.Fatalf("tools present in payload: %v", payload)
		}
		if _, found := payload["tool_choice"]; found {
			testingInstance.Fatalf("tool_choice present in payload: %v", payload)
		}
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if strings.TrimSpace(string(responseBytes)) != "NO_TOOLS_OK" {
		testingInstance.Fatalf("body=%q want %q", string(responseBytes), "NO_TOOLS_OK")
	}
}
