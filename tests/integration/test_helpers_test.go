package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

const (
	serviceSecretValue      = "sekret"
	openAIKeyValue          = "sk-test"
	mockModelsURL           = "https://mock.local/v1/models"
	mockResponsesURL        = "https://mock.local/v1/responses"
	modelsPath              = "/v1/models"
	responsesPath           = "/v1/responses"
	modelListBody           = `{"object":"list","data":[{"id":"gpt-4.1","object":"model"}]}`
	availableModelsBody     = `{"data":[{"id":"gpt-4.1"},{"id":"gpt-5-mini"}]}`
	integrationOKBody       = "INTEGRATION_OK"
	integrationSearchBody   = "SEARCH_OK"
	headerContentTypeKey    = "Content-Type"
	mimeTypeApplicationJSON = "application/json"
	logLevelDebug           = "debug"
	promptQueryParameter    = "prompt"
	keyQueryParameter       = "key"
	promptValue             = "ping"
)

type roundTripperFunc func(httpRequest *http.Request) (*http.Response, error)

func (roundTripper roundTripperFunc) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	return roundTripper(httpRequest)
}

// newOpenAIServer returns a stub OpenAI server yielding the provided body and optionally capturing requests.
func newOpenAIServer(testingInstance *testing.T, responseText string, captureTarget *any) *httptest.Server {
	testingInstance.Helper()
	handler := http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch httpRequest.URL.Path {
		case modelsPath:
			responseWriter.Header().Set(headerContentTypeKey, mimeTypeApplicationJSON)
			_, _ = io.WriteString(responseWriter, modelListBody)
		case responsesPath:
			if captureTarget != nil {
				body, _ := io.ReadAll(httpRequest.Body)
				_ = json.Unmarshal(body, captureTarget)
			}
			responseWriter.Header().Set(headerContentTypeKey, mimeTypeApplicationJSON)
			_, _ = io.WriteString(responseWriter, `{"output_text":"`+responseText+`"}`)
		default:
			http.NotFound(responseWriter, httpRequest)
		}
	})
	return httptest.NewServer(handler)
}

// newIntegrationServer builds the application server pointing at the provided OpenAI server.
func newIntegrationServer(testingInstance *testing.T, openAIServer *httptest.Server) *httptest.Server {
	testingInstance.Helper()
	proxy.SetModelsURL(openAIServer.URL + modelsPath)
	proxy.SetResponsesURL(openAIServer.URL + responsesPath)
	proxy.HTTPClient = openAIServer.Client()
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)
	router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: serviceSecretValue,
		OpenAIKey:     openAIKeyValue,
		LogLevel:      logLevelDebug,
		WorkerCount:   1,
		QueueSize:     4,
	}, newLogger(testingInstance))
	if buildRouterError != nil {
		testingInstance.Fatalf("BuildRouter error: %v", buildRouterError)
	}
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)
	return server
}

// newIntegrationServerWithTimeout builds the application server with a configurable request timeout.
func newIntegrationServerWithTimeout(testingInstance *testing.T, openAIServer *httptest.Server, requestTimeoutSeconds int) *httptest.Server {
	testingInstance.Helper()
	proxy.SetModelsURL(openAIServer.URL + modelsPath)
	proxy.SetResponsesURL(openAIServer.URL + responsesPath)
	proxy.HTTPClient = openAIServer.Client()
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)
	router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:         serviceSecretValue,
		OpenAIKey:             openAIKeyValue,
		LogLevel:              logLevelDebug,
		WorkerCount:           1,
		QueueSize:             4,
		RequestTimeoutSeconds: requestTimeoutSeconds,
	}, newLogger(testingInstance))
	if buildRouterError != nil {
		testingInstance.Fatalf("BuildRouter error: %v", buildRouterError)
	}
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)
	return server
}

// makeHTTPClient returns a stub HTTP client capturing payloads and returning canned responses.
func makeHTTPClient(testingInstance *testing.T, wantWebSearch bool) (*http.Client, *map[string]any) {
	testingInstance.Helper()
	var captured map[string]any
	client := &http.Client{
		Transport: roundTripperFunc(func(httpRequest *http.Request) (*http.Response, error) {
			switch httpRequest.URL.String() {
			case proxy.ModelsURL():
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(availableModelsBody)), Header: make(http.Header)}, nil
			case proxy.ResponsesURL():
				if httpRequest.Body != nil {
					buf, _ := io.ReadAll(httpRequest.Body)
					_ = json.Unmarshal(buf, &captured)
				}
				text := integrationOKBody
				if wantWebSearch {
					text = integrationSearchBody
				}
				body := `{"output_text":"` + text + `"}`
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			default:
				testingInstance.Fatalf("unexpected request to %s", httpRequest.URL.String())
				return nil, nil
			}
		}),
		Timeout: 5 * time.Second,
	}
	return client, &captured
}

// configureProxy sets URLs and the HTTP client for proxy operations.
func configureProxy(testingInstance *testing.T, client *http.Client) {
	testingInstance.Helper()
	proxy.HTTPClient = client
	proxy.SetModelsURL(mockModelsURL)
	proxy.SetResponsesURL(mockResponsesURL)
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)
}

// newLogger constructs a development logger for tests.
func newLogger(testingInstance *testing.T) *zap.SugaredLogger {
	testingInstance.Helper()
	logger, _ := zap.NewDevelopment()
	testingInstance.Cleanup(func() { _ = logger.Sync() })
	return logger.Sugar()
}
