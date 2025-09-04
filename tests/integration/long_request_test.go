package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/proxy"
)

const (
	serviceSecretValue           = "sekret"
	openAIKeyValue               = "sk-test"
	mockModelsURL                = "https://mock.local/v1/models"
	mockResponsesURL             = "https://mock.local/v1/responses"
	modelsListBody               = `{"data":[{"id":"gpt-4.1"}]}`
	expectedResponseBody         = "SLOW_OK"
	promptQueryParameter         = "prompt"
	keyQueryParameter            = "key"
	promptValue                  = "ping"
	responseDelay                = 31 * time.Second
	httpClientTimeout            = responseDelay + 5*time.Second
	requestTimeoutSecondsDefault = 40
)

// makeSlowHTTPClient returns an HTTP client that simulates a delayed upstream response.
func makeSlowHTTPClient(testingInstance *testing.T) *http.Client {
	testingInstance.Helper()
	return &http.Client{
		Transport: rt(func(request *http.Request) (*http.Response, error) {
			switch request.URL.String() {
			case proxy.ModelsURL():
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(modelsListBody)), Header: make(http.Header)}, nil
			case proxy.ResponsesURL():
				time.Sleep(responseDelay)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"output_text":"` + expectedResponseBody + `"}`)), Header: make(http.Header)}, nil
			default:
				testingInstance.Fatalf("unexpected request to %s", request.URL.String())
				return nil, nil
			}
		}),
		Timeout: httpClientTimeout,
	}
}

// TestIntegration_ResponseDeliveredAfterDelay verifies responses are sent after long upstream delays.
func TestIntegration_ResponseDeliveredAfterDelay(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	proxy.HTTPClient = makeSlowHTTPClient(testingInstance)
	proxy.SetModelsURL(mockModelsURL)
	proxy.SetResponsesURL(mockResponsesURL)
	router, buildError := proxy.BuildRouter(proxy.Configuration{ServiceSecret: serviceSecretValue, OpenAIKey: openAIKeyValue, LogLevel: "debug", WorkerCount: 1, QueueSize: 8, RequestTimeoutSeconds: requestTimeoutSecondsDefault}, newLogger(testingInstance))
	if buildError != nil {
		testingInstance.Fatalf("BuildRouter failed: %v", buildError)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set(promptQueryParameter, promptValue)
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	requestURL.RawQuery = queryValues.Encode()
	httpResponse, requestError := http.Get(requestURL.String())
	if requestError != nil {
		testingInstance.Fatalf("GET failed: %v", requestError)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK {
		testingInstance.Fatalf("status=%d want=%d", httpResponse.StatusCode, http.StatusOK)
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if string(responseBytes) != expectedResponseBody {
		testingInstance.Fatalf("body=%q want=%q", string(responseBytes), expectedResponseBody)
	}
}
