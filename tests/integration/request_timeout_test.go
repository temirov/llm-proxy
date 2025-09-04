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
	timeoutServiceSecret      = "sekret"
	timeoutOpenAIKey          = "sk-test"
	timeoutModelsURL          = "https://mock.local/v1/models"
	timeoutResponsesURL       = "https://mock.local/v1/responses"
	timeoutModelsListBody     = `{"data":[{"id":"gpt-4.1"}]}`
	timeoutPromptParameter    = "prompt"
	timeoutKeyParameter       = "key"
	timeoutPromptValue        = "ping"
	timeoutExpectedStatusCode = http.StatusGatewayTimeout
	timeoutRequestTimeout     = 1
	timeoutUpstreamDelay      = 3 * time.Second
	timeoutHTTPClientTimeout  = timeoutUpstreamDelay + 2*time.Second
)

// makeTimeoutHTTPClient returns an HTTP client whose responses delay longer than the request timeout.
func makeTimeoutHTTPClient(testingInstance *testing.T) *http.Client {
	testingInstance.Helper()
	return &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			switch request.URL.String() {
			case proxy.ModelsURL():
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(timeoutModelsListBody)), Header: make(http.Header)}, nil
			case proxy.ResponsesURL():
				select {
				case <-request.Context().Done():
					return nil, request.Context().Err()
				case <-time.After(timeoutUpstreamDelay):
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"output_text":"NEVER"}`)), Header: make(http.Header)}, nil
				}
			default:
				testingInstance.Fatalf("unexpected request to %s", request.URL.String())
				return nil, nil
			}
		}),
		Timeout: timeoutHTTPClientTimeout,
	}
}

// TestIntegration_UpstreamRequestTimeoutTriggersGatewayTimeout verifies upstream timeouts result in a gateway timeout before the upstream delay elapses.
func TestIntegration_UpstreamRequestTimeoutTriggersGatewayTimeout(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	proxy.HTTPClient = makeTimeoutHTTPClient(testingInstance)
	proxy.SetModelsURL(timeoutModelsURL)
	proxy.SetResponsesURL(timeoutResponsesURL)
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)

	router, buildError := proxy.BuildRouter(proxy.Configuration{ServiceSecret: timeoutServiceSecret, OpenAIKey: timeoutOpenAIKey, LogLevel: "debug", WorkerCount: 1, QueueSize: 8, RequestTimeoutSeconds: timeoutRequestTimeout}, newLogger(testingInstance))
	if buildError != nil {
		testingInstance.Fatalf("BuildRouter failed: %v", buildError)
	}
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set(timeoutPromptParameter, timeoutPromptValue)
	queryValues.Set(timeoutKeyParameter, timeoutServiceSecret)
	requestURL.RawQuery = queryValues.Encode()

	startInstant := time.Now()
	httpResponse, requestError := http.Get(requestURL.String())
	elapsedDuration := time.Since(startInstant)
	if requestError != nil {
		testingInstance.Fatalf("GET failed: %v", requestError)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != timeoutExpectedStatusCode {
		testingInstance.Fatalf("status=%d want=%d", httpResponse.StatusCode, timeoutExpectedStatusCode)
	}
	if elapsedDuration >= timeoutUpstreamDelay {
		testingInstance.Fatalf("elapsed=%v exceeds upstream delay %v", elapsedDuration, timeoutUpstreamDelay)
	}
}
