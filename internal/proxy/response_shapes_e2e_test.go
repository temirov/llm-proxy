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

	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		if httpRequest.Method == http.MethodPost {
			// Return the initial response, which might be the job ID or the full response.
			_, _ = responseWriter.Write([]byte(initialResponse))
		} else if httpRequest.Method == http.MethodGet && strings.HasSuffix(httpRequest.URL.Path, jobID) {
			// Return the final response on poll.
			_, _ = responseWriter.Write([]byte(finalResponse))
		}
	}))
	t.Cleanup(server.Close)

	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(server.URL)

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
		Endpoints:                  endpoints,
	}, logger.Sugar())
	if err != nil {
		t.Fatalf("BuildRouter error: %v", err)
	}
	return router
}

// doRequest issues a request to the handler and returns the status code and response body.
func doRequest(t *testing.T, handler http.Handler) (int, string) {
	const (
		queryParamPrompt = "prompt"
		queryParamModel  = "model"
		queryParamKey    = "key"
	)

	queryParameters := url.Values{}
	queryParameters.Set(queryParamPrompt, TestPrompt)
	queryParameters.Set(queryParamModel, TestModel)
	queryParameters.Set(queryParamKey, TestSecret)

	req := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
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
			finalResponse: `{"status":"completed", "output":[{"type":"message", "role":"assistant", "content":[{"type":"text", "text":"Message Answer"}]}]}`,
			wantBody:      "Message Answer",
		},
		{
			name:          "fallback to web search query",
			finalResponse: `{"status":"completed", "output":[{"type":"web_search_call", "action":{"query":"final query"}}]}`,
			wantBody:      `Model did not provide a final answer. Last web search: "final query"`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := withStubbedProxy(t, initialPollResponse, testCase.finalResponse)
			status, body := doRequest(t, handler)
			if status != http.StatusOK {
				t.Fatalf("status=%d want=%d body=%s", status, http.StatusOK, body)
			}
			if body != testCase.wantBody {
				t.Fatalf("got body %q want %q", body, testCase.wantBody)
			}
		})
	}
}
