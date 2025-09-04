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

// roundTripper to stub both /models and /responses.
type rt func(req *http.Request) (*http.Response, error)

func (f rt) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func makeHTTPClient(t *testing.T, wantWebSearch bool) (*http.Client, *map[string]any) {
	t.Helper()
	var captured map[string]any

	return &http.Client{
		Transport: rt(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case proxy.ModelsURL:
				// Return known models so validator passes for tests using gpt-4.1 and gpt-5-mini.
				body := `{"data":[{"id":"gpt-4.1"},{"id":"gpt-5-mini"}]}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			case proxy.ResponsesURL:
				// Capture JSON payload to assert tools presence.
				if req.Body != nil {
					buf, _ := io.ReadAll(req.Body)
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
				t.Fatalf("unexpected request to %s", req.URL.String())
				return nil, nil
			}
		}),
		Timeout: 5 * time.Second,
	}, &captured
}

func newLogger(t *testing.T) *zap.SugaredLogger {
	t.Helper()
	l, _ := zap.NewDevelopment()
	t.Cleanup(func() { _ = l.Sync() })
	return l.Sugar()
}

func TestIntegration_ResponseDelivered_Plain(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Inject stubbed client/URLs.
	proxy.HTTPClient, _ = makeHTTPClient(t, false)
	proxy.ModelsURL = "https://mock.local/v1/models"
	proxy.ResponsesURL = "https://mock.local/v1/responses"

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     8,
	}, newLogger(t))
	if err != nil {
		t.Fatalf("BuildRouter failed: %v", err)
	}

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	q := u.Query()
	q.Set("prompt", "ping")
	q.Set("key", "sekret")
	u.RawQuery = q.Encode()

	res, err := http.Get(u.String())
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want=%d", res.StatusCode, http.StatusOK)
	}
	b, _ := io.ReadAll(res.Body)
	if string(b) != "INTEGRATION_OK" {
		t.Fatalf("body=%q want=%q", string(b), "INTEGRATION_OK")
	}
}

func TestIntegration_WebSearch_SendsTool(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client, captured := makeHTTPClient(t, true)
	proxy.HTTPClient = client
	proxy.ModelsURL = "https://mock.local/v1/models"
	proxy.ResponsesURL = "https://mock.local/v1/responses"

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     8,
	}, newLogger(t))
	if err != nil {
		t.Fatalf("BuildRouter failed: %v", err)
	}

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	q := u.Query()
	q.Set("prompt", "ping")
	q.Set("key", "sekret")
	q.Set("web_search", "1")
	u.RawQuery = q.Encode()

	res, err := http.Get(u.String())
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want=%d", res.StatusCode, http.StatusOK)
	}
	b, _ := io.ReadAll(res.Body)
	if string(b) != "SEARCH_OK" {
		t.Fatalf("body=%q want=%q", string(b), "SEARCH_OK")
	}

	// Assert tool was sent.
	tools, ok := (*captured)["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("tools missing in payload when web_search=1; captured=%v", *captured)
	}
	first, _ := tools[0].(map[string]any)
	if first["type"] != "web_search" {
		t.Fatalf("tool type=%v want=web_search", first["type"])
	}
}

func TestIntegration_RejectsWrongKeyAndMissingSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// First, BuildRouter should fail if missing secrets.
	_, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "",
		OpenAIKey:     "sk-test",
	}, newLogger(t))
	if err == nil || !strings.Contains(err.Error(), "SERVICE_SECRET") {
		t.Fatalf("expected SERVICE_SECRET error, got %v", err)
	}
	_, err = proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "",
	}, newLogger(t))
	if err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("expected OPENAI_API_KEY error, got %v", err)
	}

	// With correct config, wrong key should 403.
	proxy.HTTPClient, _ = makeHTTPClient(t, false)
	proxy.ModelsURL = "https://mock.local/v1/models"
	proxy.ResponsesURL = "https://mock.local/v1/responses"

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     4,
	}, newLogger(t))
	if err != nil {
		t.Fatalf("BuildRouter failed: %v", err)
	}
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	q := u.Query()
	q.Set("prompt", "ping")
	q.Set("key", "wrong")
	u.RawQuery = q.Encode()

	res, err := http.Get(u.String())
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, res.Body)
		t.Fatalf("status=%d want=%d body=%q", res.StatusCode, http.StatusForbidden, buf.String())
	}
}
