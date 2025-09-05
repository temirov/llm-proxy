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
	modelsListBody               = `{"data":[{"id":"` + proxy.ModelNameGPT41 + `"}]}`
	expectedResponseBody         = "SLOW_OK"
	responseDelay                = 31 * time.Second
	httpClientTimeout            = responseDelay + 5*time.Second
	requestTimeoutSecondsDefault = 40
)

// makeSlowHTTPClient returns an HTTP client that simulates a delayed upstream response.
func makeSlowHTTPClient(testingInstance *testing.T) *http.Client {
	testingInstance.Helper()
	return &http.Client{
		Transport: roundTripperFunc(func(httpRequest *http.Request) (*http.Response, error) {
			switch {
			case httpRequest.URL.String() == proxy.ModelsURL():
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(modelsListBody)), Header: make(http.Header)}, nil
			case strings.HasPrefix(httpRequest.URL.String(), proxy.ModelsURL()+"/"):
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(metadataTemperatureTools)), Header: make(http.Header)}, nil
			case httpRequest.URL.String() == proxy.ResponsesURL():
				time.Sleep(responseDelay)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"output_text":"` + expectedResponseBody + `"}`)), Header: make(http.Header)}, nil
			default:
				testingInstance.Fatalf(unexpectedRequestFormat, httpRequest.URL.String())
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
				subTest.Fatalf(buildRouterFailedFormat, buildError)
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
				subTest.Fatalf(getFailedFormat, requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != http.StatusOK {
				subTest.Fatalf(statusWantFormat, httpResponse.StatusCode, http.StatusOK)
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if string(responseBytes) != expectedResponseBody {
				subTest.Fatalf(bodyMismatchFormat, string(responseBytes), expectedResponseBody)
			}
		})
	}
}
