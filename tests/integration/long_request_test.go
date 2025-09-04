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
	serviceSecretValue = "sekret"
	openAIKeyValue     = "sk-test"
	// logLevelDebug is the logging level used in integration tests.
	logLevelDebug                = "debug"
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
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
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

// TestIntegrationResponseDeliveredAfterDelay verifies responses are sent after long upstream delays.
func TestIntegrationResponseDeliveredAfterDelay(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	testCases := []struct{ name string }{{name: "delayed_response"}}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			configureProxy(subTest, makeSlowHTTPClient(subTest))
			router, buildError := proxy.BuildRouter(proxy.Configuration{ServiceSecret: serviceSecretValue, OpenAIKey: openAIKeyValue, LogLevel: logLevelDebug, WorkerCount: 1, QueueSize: 8, RequestTimeoutSeconds: requestTimeoutSecondsDefault}, newLogger(subTest))
			if buildError != nil {
				subTest.Fatalf("BuildRouter failed: %v", buildError)
			}
			server := httptest.NewServer(router)
			subTest.Cleanup(server.Close)
			requestURL, _ := url.Parse(server.URL)
			queryValues := requestURL.Query()
			queryValues.Set(promptQueryParameter, promptValue)
			queryValues.Set(keyQueryParameter, serviceSecretValue)
			requestURL.RawQuery = queryValues.Encode()
			httpResponse, requestError := http.Get(requestURL.String())
			if requestError != nil {
				subTest.Fatalf("GET failed: %v", requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != http.StatusOK {
				subTest.Fatalf("status=%d want=%d", httpResponse.StatusCode, http.StatusOK)
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if string(responseBytes) != expectedResponseBody {
				subTest.Fatalf("body=%q want=%q", string(responseBytes), expectedResponseBody)
			}
		})
	}
}
