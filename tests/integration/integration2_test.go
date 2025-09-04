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

func (roundTripper roundTripperFunc) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	return roundTripper(httpRequest)
}

func makeHTTPClient(testingInstance *testing.T, wantWebSearch bool) (*http.Client, *map[string]any) {
	testingInstance.Helper()
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
				testingInstance.Fatalf("unexpected request to %s", httpRequest.URL.String())
				return nil, nil
			}
		}),
		Timeout: 5 * time.Second,
	}, &captured
}

func newLogger(testingInstance *testing.T) *zap.SugaredLogger {
	testingInstance.Helper()
	l, _ := zap.NewDevelopment()
	testingInstance.Cleanup(func() { _ = l.Sync() })
	return l.Sugar()
}

// TestIntegration_ResponseDelivered_Plain validates basic response delivery without web search.
func TestIntegration_ResponseDelivered_Plain(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)

	proxy.HTTPClient, _ = makeHTTPClient(testingInstance, false)
	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     8,
	}, newLogger(testingInstance))
	if err != nil {
		testingInstance.Fatalf("BuildRouter failed: %v", err)
	}

	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set("prompt", "ping")
	queryValues.Set("key", "sekret")
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
	if string(responseBytes) != "INTEGRATION_OK" {
		testingInstance.Fatalf("body=%q want=%q", string(responseBytes), "INTEGRATION_OK")
	}
}

// TestIntegration_WebSearch_SendsTool confirms that web search requests include the correct tool in the payload.
func TestIntegration_WebSearch_SendsTool(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)

	client, captured := makeHTTPClient(testingInstance, true)
	proxy.HTTPClient = client
	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     8,
	}, newLogger(testingInstance))
	if err != nil {
		testingInstance.Fatalf("BuildRouter failed: %v", err)
	}

	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set("prompt", "ping")
	queryValues.Set("key", "sekret")
	queryValues.Set("web_search", "1")
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
	if string(responseBytes) != "SEARCH_OK" {
		testingInstance.Fatalf("body=%q want=%q", string(responseBytes), "SEARCH_OK")
	}

	// Assert tool was sent.
	tools, ok := (*captured)["tools"].([]any)
	if !ok || len(tools) == 0 {
		testingInstance.Fatalf("tools missing in payload when web_search=1; captured=%v", *captured)
	}
	first, _ := tools[0].(map[string]any)
	if first["type"] != "web_search" {
		testingInstance.Fatalf("tool type=%v want=web_search", first["type"])
	}
}

// TestIntegration_RejectsWrongKeyAndMissingSecrets ensures that configuration errors and wrong API keys are handled correctly.
func TestIntegration_RejectsWrongKeyAndMissingSecrets(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)

	_, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "",
		OpenAIKey:     "sk-test",
	}, newLogger(testingInstance))
	if err == nil || !strings.Contains(err.Error(), "SERVICE_SECRET") {
		testingInstance.Fatalf("expected SERVICE_SECRET error, got %v", err)
	}
	_, err = proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "",
	}, newLogger(testingInstance))
	if err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		testingInstance.Fatalf("expected OPENAI_API_KEY error, got %v", err)
	}

	proxy.HTTPClient, _ = makeHTTPClient(testingInstance, false)
	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     4,
	}, newLogger(testingInstance))
	if err != nil {
		testingInstance.Fatalf("BuildRouter failed: %v", err)
	}
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set("prompt", "ping")
	queryValues.Set("key", "wrong")
	requestURL.RawQuery = queryValues.Encode()

	httpResponse, requestError := http.Get(requestURL.String())
	if requestError != nil {
		testingInstance.Fatalf("GET failed: %v", requestError)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusForbidden {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, httpResponse.Body)
		testingInstance.Fatalf("status=%d want=%d body=%q", httpResponse.StatusCode, http.StatusForbidden, buf.String())
	}
}
