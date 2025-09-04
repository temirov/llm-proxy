package integration_test

import (
	"bytes"
	"encoding/json"
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

// roundTripperFunc stubs both /models and /responses.
type roundTripperFunc func(httpRequest *http.Request) (*http.Response, error)

func (transport roundTripperFunc) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	return transport(httpRequest)
}

func makeHTTPClient(testingContext *testing.T, wantWebSearch bool) (*http.Client, *map[string]any) {
	testingContext.Helper()
	var captured map[string]any

	return &http.Client{
		Transport: roundTripperFunc(func(httpRequest *http.Request) (*http.Response, error) {
			switch httpRequest.URL.String() {
			case proxy.ModelsURL():
				// Return known models so validator passes for tests using gpt-4.1 and gpt-5-mini.
				body := `{"data":[{"id":"gpt-4.1"},{"id":"gpt-5-mini"}]}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			case proxy.ResponsesURL():
				// Capture JSON payload to assert tools presence.
				if httpRequest.Body != nil {
					buf, _ := io.ReadAll(httpRequest.Body)
					_ = json.Unmarshal(buf, &captured)
				}
				// Different body based on whether caller asked for search.
				text := "INTEGRATION_OK"
				if wantWebSearch {
					text = "SEARCH_OK"
				}
				body := `{"output_text":"` + text + `"}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			default:
				// If an unexpected URL is hit, fail loudly.
				testingContext.Fatalf("unexpected request to %s", httpRequest.URL.String())
				return nil, nil
			}
		}),
		Timeout: 5 * time.Second,
	}, &captured
}

func newLogger(testingContext *testing.T) *zap.SugaredLogger {
	testingContext.Helper()
	logger, _ := zap.NewDevelopment()
	testingContext.Cleanup(func() { _ = logger.Sync() })
	return logger.Sugar()
}

func TestIntegration_ResponseDelivered_Plain(testingContext *testing.T) {
	gin.SetMode(gin.TestMode)

	proxy.HTTPClient, _ = makeHTTPClient(testingContext, false)
	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	testingContext.Cleanup(proxy.ResetModelsURL)
	testingContext.Cleanup(proxy.ResetResponsesURL)

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     8,
	}, newLogger(testingContext))
	if err != nil {
		testingContext.Fatalf("BuildRouter failed: %v", err)
	}

	testServer := httptest.NewServer(router)
	testingContext.Cleanup(testServer.Close)

	parsedURL, _ := url.Parse(testServer.URL)
	queryValues := parsedURL.Query()
	queryValues.Set("prompt", "ping")
	queryValues.Set("key", "sekret")
	parsedURL.RawQuery = queryValues.Encode()

	httpResponse, err := http.Get(parsedURL.String())
	if err != nil {
		testingContext.Fatalf("GET failed: %v", err)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK {
		testingContext.Fatalf("status=%d want=%d", httpResponse.StatusCode, http.StatusOK)
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if string(responseBytes) != "INTEGRATION_OK" {
		testingContext.Fatalf("body=%q want=%q", string(responseBytes), "INTEGRATION_OK")
	}
}

func TestIntegration_WebSearch_SendsTool(testingContext *testing.T) {
	gin.SetMode(gin.TestMode)

	client, captured := makeHTTPClient(testingContext, true)
	proxy.HTTPClient = client
	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	testingContext.Cleanup(proxy.ResetModelsURL)
	testingContext.Cleanup(proxy.ResetResponsesURL)

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     8,
	}, newLogger(testingContext))
	if err != nil {
		testingContext.Fatalf("BuildRouter failed: %v", err)
	}

	testServer := httptest.NewServer(router)
	testingContext.Cleanup(testServer.Close)

	parsedURL, _ := url.Parse(testServer.URL)
	queryValues := parsedURL.Query()
	queryValues.Set("prompt", "ping")
	queryValues.Set("key", "sekret")
	queryValues.Set("web_search", "1")
	parsedURL.RawQuery = queryValues.Encode()

	httpResponse, err := http.Get(parsedURL.String())
	if err != nil {
		testingContext.Fatalf("GET failed: %v", err)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK {
		testingContext.Fatalf("status=%d want=%d", httpResponse.StatusCode, http.StatusOK)
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if string(responseBytes) != "SEARCH_OK" {
		testingContext.Fatalf("body=%q want=%q", string(responseBytes), "SEARCH_OK")
	}

	// Assert tool was sent.
	tools, toolsFound := (*captured)["tools"].([]any)
	if !toolsFound || len(tools) == 0 {
		testingContext.Fatalf("tools missing in payload when web_search=1; captured=%v", *captured)
	}
	firstTool, _ := tools[0].(map[string]any)
	if firstTool["type"] != "web_search" {
		testingContext.Fatalf("tool type=%v want=web_search", firstTool["type"])
	}
}

func TestIntegration_RejectsWrongKeyAndMissingSecrets(testingContext *testing.T) {
	gin.SetMode(gin.TestMode)

	// First, BuildRouter should fail if missing secrets.
	_, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "",
		OpenAIKey:     "sk-test",
	}, newLogger(testingContext))
	if err == nil || !strings.Contains(err.Error(), "SERVICE_SECRET") {
		testingContext.Fatalf("expected SERVICE_SECRET error, got %v", err)
	}
	_, err = proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "",
	}, newLogger(testingContext))
	if err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		testingContext.Fatalf("expected OPENAI_API_KEY error, got %v", err)
	}

	// With correct config, wrong key should return forbidden status.
	proxy.HTTPClient, _ = makeHTTPClient(testingContext, false)
	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	testingContext.Cleanup(proxy.ResetModelsURL)
	testingContext.Cleanup(proxy.ResetResponsesURL)

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     4,
	}, newLogger(testingContext))
	if err != nil {
		testingContext.Fatalf("BuildRouter failed: %v", err)
	}
	testServer := httptest.NewServer(router)
	testingContext.Cleanup(testServer.Close)

	parsedURL, _ := url.Parse(testServer.URL)
	queryValues := parsedURL.Query()
	queryValues.Set("prompt", "ping")
	queryValues.Set("key", "wrong")
	parsedURL.RawQuery = queryValues.Encode()

	httpResponse, err := http.Get(parsedURL.String())
	if err != nil {
		testingContext.Fatalf("GET failed: %v", err)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusForbidden {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, httpResponse.Body)
		testingContext.Fatalf("status=%d want=%d body=%q", httpResponse.StatusCode, http.StatusForbidden, buffer.String())
	}
}
