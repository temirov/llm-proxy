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

type adaptiveRoundTripper func(httpRequest *http.Request) (*http.Response, error)

func (roundTripper adaptiveRoundTripper) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	return roundTripper(httpRequest)
}

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
				case "temperature":
					if strings.Contains(payload, `"temperature"`) {
						errBody := `{"error":{"message":"Unsupported parameter: 'temperature' is not supported with this model.","type":"invalid_request_error","param":"temperature","code":null}}`
						return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(errBody)), Header: make(http.Header)}, nil
					}
					ok := `{"output_text":"ADAPT_OK_NO_TEMP"}`
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(ok)), Header: make(http.Header)}, nil
				case "tools":
					if strings.Contains(payload, `"tools"`) {
						errBody := `{"error":{"message":"Unsupported parameter: 'tools' is not supported with this model.","type":"invalid_request_error","param":"tools","code":null}}`
						return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(errBody)), Header: make(http.Header)}, nil
					}
					ok := `{"output_text":"ADAPT_OK_NO_TOOLS"}`
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
func newAdaptiveRouter(testingInstance *testing.T, mode string) *gin.Engine {
	testingInstance.Helper()
	gin.SetMode(gin.TestMode)
	proxy.HTTPClient = newAdaptiveClient(testingInstance, mode)
	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)

	logger, _ := zap.NewDevelopment()
	testingInstance.Cleanup(func() { _ = logger.Sync() })
	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     8,
	}, logger.Sugar())
	if err != nil {
		testingInstance.Fatalf("BuildRouter failed: %v", err)
	}
	return router
}

// TestAdaptive_RemovesTemperatureOn400 ensures that temperature is removed after a 400 response.
func TestAdaptive_RemovesTemperatureOn400(testingInstance *testing.T) {
	router := newAdaptiveRouter(testingInstance, "temperature")
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set("prompt", "ping")
	queryValues.Set("key", "sekret")
	queryValues.Set("model", "gpt-5-mini")
	requestURL.RawQuery = queryValues.Encode()

	httpResponse, requestError := http.Get(requestURL.String())
	if requestError != nil {
		testingInstance.Fatalf("GET failed: %v", requestError)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, httpResponse.Body)
		testingInstance.Fatalf("status=%d body=%q", httpResponse.StatusCode, buf.String())
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if string(responseBytes) != "ADAPT_OK_NO_TEMP" {
		testingInstance.Fatalf("body=%q want=%q", string(responseBytes), "ADAPT_OK_NO_TEMP")
	}
}

// TestAdaptive_RemovesToolsOn400 ensures that tools are removed after a 400 response when web search is enabled.
func TestAdaptive_RemovesToolsOn400(testingInstance *testing.T) {
	router := newAdaptiveRouter(testingInstance, "tools")
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set("prompt", "ping")
	queryValues.Set("key", "sekret")
	queryValues.Set("model", "gpt-5-mini")
	queryValues.Set("web_search", "1")
	requestURL.RawQuery = queryValues.Encode()

	httpResponse, requestError := http.Get(requestURL.String())
	if requestError != nil {
		testingInstance.Fatalf("GET failed: %v", requestError)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, httpResponse.Body)
		testingInstance.Fatalf("status=%d body=%q", httpResponse.StatusCode, buf.String())
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if string(responseBytes) != "ADAPT_OK_NO_TOOLS" {
		testingInstance.Fatalf("body=%q want=%q", string(responseBytes), "ADAPT_OK_NO_TOOLS")
	}
}
