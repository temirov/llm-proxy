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

func newAdaptiveClient(testingContext *testing.T, mode string) *http.Client {
	testingContext.Helper()
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
					successPayload := `{"output_text":"ADAPT_OK_NO_TEMP"}`
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(successPayload)), Header: make(http.Header)}, nil
				case "tools":
					if strings.Contains(payload, `"tools"`) {
						errBody := `{"error":{"message":"Unsupported parameter: 'tools' is not supported with this model.","type":"invalid_request_error","param":"tools","code":null}}`
						return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(errBody)), Header: make(http.Header)}, nil
					}
					successPayload := `{"output_text":"ADAPT_OK_NO_TOOLS"}`
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(successPayload)), Header: make(http.Header)}, nil
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

func newAdaptiveRouter(testingContext *testing.T, mode string) *gin.Engine {
	testingContext.Helper()
	gin.SetMode(gin.TestMode)
	proxy.HTTPClient = newAdaptiveClient(testingContext, mode)
	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	testingContext.Cleanup(proxy.ResetModelsURL)
	testingContext.Cleanup(proxy.ResetResponsesURL)

	logger, _ := zap.NewDevelopment()
	testingContext.Cleanup(func() { _ = logger.Sync() })
	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     8,
	}, logger.Sugar())
	if err != nil {
		testingContext.Fatalf("BuildRouter failed: %v", err)
	}
	return router
}

func TestAdaptive_RemovesTemperatureOn400(testingContext *testing.T) {
	router := newAdaptiveRouter(testingContext, "temperature")
	testServer := httptest.NewServer(router)
	testingContext.Cleanup(testServer.Close)

	parsedURL, _ := url.Parse(testServer.URL)
	queryValues := parsedURL.Query()
	queryValues.Set("prompt", "ping")
	queryValues.Set("key", "sekret")
	queryValues.Set("model", "gpt-5-mini")
	parsedURL.RawQuery = queryValues.Encode()

	httpResponse, err := http.Get(parsedURL.String())
	if err != nil {
		testingContext.Fatalf("GET failed: %v", err)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, httpResponse.Body)
		testingContext.Fatalf("status=%d body=%q", httpResponse.StatusCode, buffer.String())
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if string(responseBytes) != "ADAPT_OK_NO_TEMP" {
		testingContext.Fatalf("body=%q want=%q", string(responseBytes), "ADAPT_OK_NO_TEMP")
	}
}

func TestAdaptive_RemovesToolsOn400(testingContext *testing.T) {
	router := newAdaptiveRouter(testingContext, "tools")
	testServer := httptest.NewServer(router)
	testingContext.Cleanup(testServer.Close)

	parsedURL, _ := url.Parse(testServer.URL)
	queryValues := parsedURL.Query()
	queryValues.Set("prompt", "ping")
	queryValues.Set("key", "sekret")
	queryValues.Set("model", "gpt-5-mini")
	queryValues.Set("web_search", "1")
	parsedURL.RawQuery = queryValues.Encode()

	httpResponse, err := http.Get(parsedURL.String())
	if err != nil {
		testingContext.Fatalf("GET failed: %v", err)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, httpResponse.Body)
		testingContext.Fatalf("status=%d body=%q", httpResponse.StatusCode, buffer.String())
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if string(responseBytes) != "ADAPT_OK_NO_TOOLS" {
		testingContext.Fatalf("body=%q want=%q", string(responseBytes), "ADAPT_OK_NO_TOOLS")
	}
}
