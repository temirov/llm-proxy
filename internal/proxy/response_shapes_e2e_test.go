package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

const (
	TestSecret  = "sekret"
	TestAPIKey  = "sk-test"
	TestPrompt  = "hello"
	TestModel   = proxy.ModelNameGPT4o
	TestTimeout = 5
)

// withStubbedProxy now uses a simple mock server for polling.
func withStubbedProxy(t *testing.T, initialResponse, finalResponse string) http.Handler {
	t.Helper()
	const jobID = "resp_test_123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			// Return the initial response, which might be the job ID or the full response.
			_, _ = w.Write([]byte(initialResponse))
		} else if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, jobID) {
			// Return the final response on poll.
			_, _ = w.Write([]byte(finalResponse))
		}
	}))
	t.Cleanup(server.Close)

	proxy.SetResponsesURL(server.URL)
	t.Cleanup(proxy.ResetResponsesURL)

	logger, _ := zap.NewDevelopment()
	t.Cleanup(func() { _ = logger.Sync() })
	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		LogLevel:                   proxy.LogLevelDebug,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if err != nil {
		t.Fatalf("BuildRouter error: %v", err)
	}
	return router
}

func doRequest(t *testing.T, handler http.Handler) (int, string) {
	q := url.Values{}
	q.Set("prompt", TestPrompt)
	q.Set("model", TestModel)
	q.Set("key", TestSecret)

	req := httptest.NewRequest(http.MethodGet, "/?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func Test_ResponseShapes(t *testing.T) {
	initialPollResponse := `{"id":"resp_test_123", "status":"queued"}`

	testCases := []struct {
		name          string
		finalResponse string
		wantBody      string
	}{
		{
			name:          "simple output_text field",
			finalResponse: `{"status":"completed", "output_text":"Simple Answer"}`,
			wantBody:      "Simple Answer",
		},
		{
			name:          "message object in output array",
			finalResponse: `{"status":"completed", "output":[{"type":"message", "role":"assistant", "content":[{"type":"output_text", "text":"Message Answer"}]}]}`,
			wantBody:      "Message Answer",
		},
		{
			name:          "fallback to web search query",
			finalResponse: `{"status":"completed", "output":[{"type":"web_search_call", "action":{"query":"final query"}}]}`,
			wantBody:      `Model did not provide a final answer. Last web search: "final query"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := withStubbedProxy(t, initialPollResponse, tc.finalResponse)
			status, body := doRequest(t, handler)
			if status != http.StatusOK {
				t.Fatalf("status=%d want=%d body=%s", status, http.StatusOK, body)
			}
			if body != tc.wantBody {
				t.Fatalf("got body %q want %q", body, tc.wantBody)
			}
		})
	}
}
