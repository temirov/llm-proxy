package integration_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

const (
	adaptiveModelQueryParameter = "model"
	adaptiveModeTemperature     = "temperature"
	adaptiveModeTools           = "tools"
	adaptiveOKNoTemp            = "ADAPT_OK_NO_TEMP"
	adaptiveOKNoTools           = "ADAPT_OK_NO_TOOLS"
)

type adaptiveRoundTripper func(httpRequest *http.Request) (*http.Response, error)

func (roundTripper adaptiveRoundTripper) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	return roundTripper(httpRequest)
}

// newAdaptiveClient returns an HTTP client that adapts to unsupported parameters.
func newAdaptiveClient(testingInstance *testing.T, mode string) *http.Client {
	testingInstance.Helper()
	return &http.Client{
		Transport: adaptiveRoundTripper(func(httpRequest *http.Request) (*http.Response, error) {
			switch httpRequest.URL.String() {
			case proxy.ModelsURL():
				body := `{"data":[{"id":"gpt-5-mini"}]}`
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			case proxy.ResponsesURL():
				buf, _ := io.ReadAll(httpRequest.Body)
				httpRequest.Body.Close()
				payload := string(buf)
				switch mode {
				case adaptiveModeTemperature:
					if strings.Contains(payload, `"temperature"`) {
						errBody := `{"error":{"message":"Unsupported parameter: 'temperature' is not supported with this model.","type":"invalid_request_error","param":"temperature","code":null}}`
						return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(errBody)), Header: make(http.Header)}, nil
					}
					ok := `{"output_text":"` + adaptiveOKNoTemp + `"}`
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(ok)), Header: make(http.Header)}, nil
				case adaptiveModeTools:
					if strings.Contains(payload, `"tools"`) {
						errBody := `{"error":{"message":"Unsupported parameter: 'tools' is not supported with this model.","type":"invalid_request_error","param":"tools","code":null}}`
						return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(errBody)), Header: make(http.Header)}, nil
					}
					ok := `{"output_text":"` + adaptiveOKNoTools + `"}`
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(ok)), Header: make(http.Header)}, nil
				default:
					return &http.Response{StatusCode: http.StatusTeapot, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
				}
			default:
				return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
			}
		}),
		Timeout: 5 * time.Second,
	}
}

// newAdaptiveRouter constructs a router for adaptive testing.
func newAdaptiveRouter(testingInstance *testing.T, mode string) *gin.Engine {
	testingInstance.Helper()
	gin.SetMode(gin.TestMode)
	proxy.HTTPClient = newAdaptiveClient(testingInstance, mode)
	proxy.SetModelsURL(mockModelsURL)
	proxy.SetResponsesURL(mockResponsesURL)
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)
	logger, _ := zap.NewDevelopment()
	testingInstance.Cleanup(func() { _ = logger.Sync() })
	router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: serviceSecretValue,
		OpenAIKey:     openAIKeyValue,
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     8,
	}, logger.Sugar())
	if buildRouterError != nil {
		testingInstance.Fatalf("BuildRouter failed: %v", buildRouterError)
	}
	return router
}

// TestAdaptiveRemovesUnsupportedParameters verifies adaptive removal of unsupported fields.
func TestAdaptiveRemovesUnsupportedParameters(testingInstance *testing.T) {
	testCases := []struct {
		name     string
		mode     string
		expected string
		query    map[string]string
	}{
		{
			name:     "temperature",
			mode:     adaptiveModeTemperature,
			expected: adaptiveOKNoTemp,
			query: map[string]string{
				adaptiveModelQueryParameter: "gpt-5-mini",
			},
		},
		{
			name:     "tools",
			mode:     adaptiveModeTools,
			expected: adaptiveOKNoTools,
			query: map[string]string{
				adaptiveModelQueryParameter: "gpt-5-mini",
				webSearchQueryParameter:     "1",
			},
		},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			router := newAdaptiveRouter(subTest, testCase.mode)
			server := httptest.NewServer(router)
			subTest.Cleanup(server.Close)
			requestURL, _ := url.Parse(server.URL)
			queryValues := requestURL.Query()
			queryValues.Set(promptQueryParameter, promptValue)
			queryValues.Set(keyQueryParameter, serviceSecretValue)
			for key, value := range testCase.query {
				queryValues.Set(key, value)
			}
			requestURL.RawQuery = queryValues.Encode()
			httpResponse, requestError := http.Get(requestURL.String())
			if requestError != nil {
				subTest.Fatalf("GET failed: %v", requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != http.StatusOK {
				var buf bytes.Buffer
				_, _ = io.Copy(&buf, httpResponse.Body)
				subTest.Fatalf("status=%d body=%q", httpResponse.StatusCode, buf.String())
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if string(responseBytes) != testCase.expected {
				subTest.Fatalf("body=%q want=%q", string(responseBytes), testCase.expected)
			}
		})
	}
}
