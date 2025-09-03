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

type adaptiveRoundTripper func(req *http.Request) (*http.Response, error)

func (f adaptiveRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func newAdaptiveClient(t *testing.T, mode string) *http.Client {
	t.Helper()
	return &http.Client{
		Transport: adaptiveRoundTripper(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case proxy.ModelsURL:
				body := `{"data":[{"id":"gpt-5-mini"}]}`
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			case proxy.ResponsesURL:
				buf, _ := io.ReadAll(req.Body)
				req.Body.Close()
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

func newAdaptiveRouter(t *testing.T, mode string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	proxy.HTTPClient = newAdaptiveClient(t, mode)
	proxy.ModelsURL = "https://mock.local/v1/models"
	proxy.ResponsesURL = "https://mock.local/v1/responses"

	logger, _ := zap.NewDevelopment()
	t.Cleanup(func() { _ = logger.Sync() })
	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     8,
	}, logger.Sugar())
	if err != nil {
		t.Fatalf("BuildRouter failed: %v", err)
	}
	return router
}

func TestAdaptive_RemovesTemperatureOn400(t *testing.T) {
	router := newAdaptiveRouter(t, "temperature")
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	q := u.Query()
	q.Set("prompt", "ping")
	q.Set("key", "sekret")
	q.Set("model", "gpt-5-mini")
	u.RawQuery = q.Encode()

	res, err := http.Get(u.String())
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, res.Body)
		t.Fatalf("status=%d body=%q", res.StatusCode, buf.String())
	}
	b, _ := io.ReadAll(res.Body)
	if string(b) != "ADAPT_OK_NO_TEMP" {
		t.Fatalf("body=%q want=%q", string(b), "ADAPT_OK_NO_TEMP")
	}
}

func TestAdaptive_RemovesToolsOn400(t *testing.T) {
	router := newAdaptiveRouter(t, "tools")
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	q := u.Query()
	q.Set("prompt", "ping")
	q.Set("key", "sekret")
	q.Set("model", "gpt-5-mini")
	q.Set("web_search", "1")
	u.RawQuery = q.Encode()

	res, err := http.Get(u.String())
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, res.Body)
		t.Fatalf("status=%d body=%q", res.StatusCode, buf.String())
	}
	b, _ := io.ReadAll(res.Body)
	if string(b) != "ADAPT_OK_NO_TOOLS" {
		t.Fatalf("body=%q want=%q", string(b), "ADAPT_OK_NO_TOOLS")
	}
}
