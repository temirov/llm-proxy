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
			switch {
			case request.URL.String() == proxy.ModelsURL():
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(modelsListBody)), Header: make(http.Header)}, nil
			case strings.HasPrefix(request.URL.String(), proxy.ModelsURL()+"/"):
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(metadataTemperatureTools)), Header: make(http.Header)}, nil
			case request.URL.String() == proxy.ResponsesURL():
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

// TestIntegrationUpstreamRequestTimeoutTriggersGatewayTimeout verifies upstream timeouts result in a gateway timeout before the upstream delay elapses.
func TestIntegrationUpstreamRequestTimeoutTriggersGatewayTimeout(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	testCases := []struct{ name string }{{name: "gateway_timeout"}}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			configureProxy(subTest, makeTimeoutHTTPClient(subTest))
			router, buildError := proxy.BuildRouter(proxy.Configuration{ServiceSecret: serviceSecretValue, OpenAIKey: openAIKeyValue, LogLevel: logLevelDebug, WorkerCount: 1, QueueSize: 8, RequestTimeoutSeconds: timeoutRequestTimeout}, newLogger(subTest))
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
			startInstant := time.Now()
			httpResponse, requestError := http.Get(requestURL.String())
			elapsedDuration := time.Since(startInstant)
			if requestError != nil {
				subTest.Fatalf("GET failed: %v", requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != timeoutExpectedStatusCode {
				subTest.Fatalf("status=%d want=%d", httpResponse.StatusCode, timeoutExpectedStatusCode)
			}
			if elapsedDuration >= timeoutUpstreamDelay {
				subTest.Fatalf("elapsed=%v exceeds upstream delay %v", elapsedDuration, timeoutUpstreamDelay)
			}
		})
	}
}
