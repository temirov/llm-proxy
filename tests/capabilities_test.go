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
	modelIDGPT4oMini = "gpt-4o-mini"
	modelIDGPT4o     = "gpt-4o"
	modelIDGPT5Mini  = "gpt-5-mini"
	serviceSecret    = "sekret"
	openAIKey        = "sk-test"
	logLevel         = "debug"
)

// TestIntegration_WebSearch_UnsupportedModel_Returns400 verifies that requesting web search for an unsupported model returns an HTTP 400 status code.
func TestIntegration_WebSearch_UnsupportedModel_Returns400(testingInstance *testing.T) {
	openAISrv := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch {
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/models"):
			io.WriteString(responseWriter, `{"data":[{"id":"`+modelIDGPT4oMini+`"},{"id":"`+modelIDGPT4o+`"}]}`)
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/responses"):
			io.WriteString(responseWriter, `{"output_text":"SHOULD_NOT_BE_CALLED"}`)
		default:
			http.NotFound(responseWriter, httpRequest)
		}
	}))
	defer openAISrv.Close()

	proxy.SetModelsURL(openAISrv.URL + "/v1/models")
	proxy.SetResponsesURL(openAISrv.URL + "/v1/responses")
	proxy.HTTPClient = openAISrv.Client()
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: serviceSecret,
		OpenAIKey:     openAIKey,
		LogLevel:      logLevel,
		WorkerCount:   1,
		QueueSize:     4,
	}, logger.Sugar())
	if err != nil {
		testingInstance.Fatalf("BuildRouter error: %v", err)
	}

	app := httptest.NewServer(router)
	defer app.Close()

	httpRequest, _ := http.NewRequest("GET", app.URL+"/?prompt=x&key="+serviceSecret+"&model="+modelIDGPT4oMini+"&web_search=1", nil)
	httpResponse, requestError := http.DefaultClient.Do(httpRequest)
	if requestError != nil {
		testingInstance.Fatalf("request failed: %v", requestError)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusBadRequest {
		testingInstance.Fatalf("status=%d want=%d", httpResponse.StatusCode, http.StatusBadRequest)
	}
	body, _ := io.ReadAll(httpResponse.Body)
	if !strings.Contains(string(body), "web_search is not supported") {
		testingInstance.Fatalf("body=%q missing capability message", string(body))
	}
}

// TestIntegration_TemperatureUnsupportedModel_RetriesWithoutTemperature confirms that requests retry without temperature for models that do not support the parameter.
func TestIntegration_TemperatureUnsupportedModel_RetriesWithoutTemperature(testingInstance *testing.T) {
	var observed any

	openAISrv := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch {
		case strings.HasSuffix(httpRequest.URL.Path, "/v1/models"):
			io.WriteString(responseWriter, `{"data":[{"id":"`+modelIDGPT5Mini+`"}]}`)
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
	defer openAISrv.Close()

	proxy.SetModelsURL(openAISrv.URL + "/v1/models")
	proxy.SetResponsesURL(openAISrv.URL + "/v1/responses")
	proxy.HTTPClient = openAISrv.Client()
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: serviceSecret,
		OpenAIKey:     openAIKey,
		LogLevel:      logLevel,
		WorkerCount:   1,
		QueueSize:     4,
	}, logger.Sugar())
	if err != nil {
		testingInstance.Fatalf("BuildRouter error: %v", err)
	}

	app := httptest.NewServer(router)
	defer app.Close()

	httpResponse, requestError := http.Get(app.URL + "/?prompt=hello&key=" + serviceSecret + "&model=" + modelIDGPT5Mini)
	if requestError != nil {
		testingInstance.Fatalf("request failed: %v", requestError)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		testingInstance.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBody))
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if strings.TrimSpace(string(responseBytes)) != "TEMPLESS_OK" {
		testingInstance.Fatalf("body=%q want %q", string(responseBytes), "TEMPLESS_OK")
	}
}
