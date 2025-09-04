package llm_proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// newRouterWithStubbedOpenAI returns a router that uses a stubbed OpenAI backend.
func newRouterWithStubbedOpenAI(t *testing.T, modelsBody, responsesBody string) *gin.Engine {
	t.Helper()

	orig := proxy.HTTPClient
	t.Cleanup(func() { proxy.HTTPClient = orig })

	proxy.HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			switch request.URL.String() {
			case proxy.ModelsURL():
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(modelsBody)),
					Header:     make(http.Header),
				}, nil
			case proxy.ResponsesURL():
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(responsesBody)),
					Header:     make(http.Header),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`{}`)),
					Header:     make(http.Header),
				}, nil
			}
		}),
		Timeout: 5 * time.Second,
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     4,
	}, logger.Sugar())
	if err != nil {
		t.Fatalf("BuildRouter error: %v", err)
	}
	return router
}

func TestEndpoint_Empty200TreatedAsError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	t.Cleanup(proxy.ResetModelsURL)
	t.Cleanup(proxy.ResetResponsesURL)

	router := newRouterWithStubbedOpenAI(
		t,
		`{"data":[{"id":"gpt-4.1"}]}`,
		`{"output":[]}`,
	)

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("GET", srv.URL+"/?prompt=test&key=sekret", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("status=%d want=%d", res.StatusCode, http.StatusBadGateway)
	}
}

func TestEndpoint_RespectsAcceptHeaderCSV(t *testing.T) {
	gin.SetMode(gin.TestMode)

	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	t.Cleanup(proxy.ResetModelsURL)
	t.Cleanup(proxy.ResetResponsesURL)

	router := newRouterWithStubbedOpenAI(
		t,
		`{"data":[{"id":"gpt-4.1"}]}`,
		`{"output_text":"Hello, world!"}`,
	)

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("GET", srv.URL+"/?prompt=anything&key=sekret", nil)
	req.Header.Set("Accept", "text/csv")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if ct := res.Header.Get("Content-Type"); ct != "text/csv" {
		t.Fatalf("content-type=%q want=%q", ct, "text/csv")
	}
	b, _ := io.ReadAll(res.Body)
	if got := string(b); got != "\"Hello, world!\"\n" {
		t.Fatalf("body=%q want=%q", got, "\"Hello, world!\"\n")
	}
}
