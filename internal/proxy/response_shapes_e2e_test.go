package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// doRequest performs a standard test request.
func doRequest(t *testing.T, handler http.Handler, key string) (int, string) {
	t.Helper()
	q := url.Values{}
	q.Set("prompt", TestPrompt)
	q.Set("model", TestModel)
	q.Set("key", key)

	req := httptest.NewRequest(http.MethodGet, "/?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func Test_ResponseShapes_EndToEnd(t *testing.T) {
	testCases := []struct {
		name       string
		openAIJSON string // This is the FINAL, polled response.
		wantBody   string
	}{
		{
			name:       "direct output_text field",
			openAIJSON: `{"status":"completed", "output_text":"Simple Answer"}`,
			wantBody:   "Simple Answer",
		},
		{
			name:       "response message content",
			openAIJSON: `{"status":"completed", "output":[{"type":"message", "role":"assistant", "content":[{"type":"output_text","text":"Alpha\nBeta"}]}]}`,
			wantBody:   "Alpha\nBeta",
		},
		{
			name:       "fallback to last web search query",
			openAIJSON: `{"status":"completed", "output":[{"type":"web_search_call","action":{"query":"final query"}}]}`,
			wantBody:   `Model did not provide a final answer. Last web search: "final query"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockServer := NewSessionMockServer(tc.openAIJSON)
			defer mockServer.Close()
			router := NewTestRouter(t, mockServer.URL)

			status, body := doRequest(t, router, TestSecret)

			if status != http.StatusOK {
				t.Fatalf("status=%d want=%d body=%s", status, http.StatusOK, body)
			}
			if body != tc.wantBody {
				t.Fatalf("got body %q want %q", body, tc.wantBody)
			}
		})
	}
}

func Test_ResponsePrecedence_EndToEnd(t *testing.T) {
	// This test verifies that a final assistant message is preferred over the fallback.
	compound := `{"status":"completed", "output":[{"type":"message", "role":"assistant", "content":[{"type":"output_text","text":"Final Answer"}]}, {"type":"web_search_call","action":{"query":"some query"}}]}`
	mockServer := NewSessionMockServer(compound)
	defer mockServer.Close()
	router := NewTestRouter(t, mockServer.URL)

	status, body := doRequest(t, router, TestSecret)

	if status != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", status, http.StatusOK, body)
	}
	if body != "Final Answer" {
		t.Fatalf("precedence wrong: got %q want %q", body, "Final Answer")
	}
}
