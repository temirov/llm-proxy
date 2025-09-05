package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

const (
	modelIdentifier = "gpt-4o"
)

// withStubbedProxy spins up stub upstream servers (models + responses),
// points the proxy at them, builds the router, and wires cleanup.
func withStubbedProxy(t *testing.T, openAIJSON string) http.Handler {
	t.Helper()

	// Stub models list: mark modelIdentifier as supported.
	modelsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"id":"`+modelIdentifier+`"}]}`)
	}))
	t.Cleanup(modelsServer.Close)

	// Stub responses API: always return the provided body (for both POST / and GET /{id}).
	responsesServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, openAIJSON)
	}))
	t.Cleanup(responsesServer.Close)

	// Point proxy at our stubs.
	proxy.SetModelsURL(modelsServer.URL)
	proxy.SetResponsesURL(responsesServer.URL)
	t.Cleanup(proxy.ResetModelsURL)
	t.Cleanup(proxy.ResetResponsesURL)

	// Build router.
	logger, _ := zap.NewDevelopment()
	t.Cleanup(func() { _ = logger.Sync() })
	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: serviceSecretValue,
		OpenAIKey:     openAIKeyValue,
		LogLevel:      proxy.LogLevelDebug,
		WorkerCount:   1,
		QueueSize:     1,
	}, logger.Sugar())
	if err != nil {
		t.Fatalf("BuildRouter error: %v", err)
	}
	return router
}

// doRequest performs GET / with the shared query params and returns (status, body).
func doRequest(t *testing.T, handler http.Handler) (int, string) {
	t.Helper()
	q := url.Values{}
	q.Set("prompt", promptValue)
	q.Set("model", modelIdentifier)
	q.Set("key", serviceSecretValue)

	req := httptest.NewRequest(http.MethodGet, "/?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func Test_ResponseShapes_EndToEnd(t *testing.T) {
	testCases := []struct {
		name       string
		openAIJSON string
		wantBody   string
	}{
		{
			name:       "direct output_text",
			openAIJSON: `{"output_text":"Alpha\nBeta"}`,
			wantBody:   "Alpha\nBeta",
		},
		{
			name:       "output nested containers",
			openAIJSON: `{"output":[{"content":[{"type":"output_text","text":"One"},{"type":"output_text","text":"Two"}]}]}`,
			wantBody:   "One\nTwo",
		},
		{
			name:       "output flat parts",
			openAIJSON: `{"output":[{"type":"output_text","text":"Flat A"},{"type":"output_text","text":"Flat B"}]}`,
			wantBody:   "Flat A\nFlat B",
		},
		{
			name:       "response nested containers (fallback)",
			openAIJSON: `{"response":[{"content":[{"type":"output_text","text":"R1"},{"type":"output_text","text":"R2"}]}]}`,
			wantBody:   "R1\nR2",
		},
		{
			name:       "response flat parts (fallback)",
			openAIJSON: `{"response":[{"type":"output_text","text":"RX"},{"type":"output_text","text":"RY"}]}`,
			wantBody:   "RX\nRY",
		},
		{
			name:       "choices message content (legacy fallback)",
			openAIJSON: `{"choices":[{"message":{"content":"Legacy"}}]}`,
			wantBody:   "Legacy",
		},
	}

	for _, current := range testCases {
		t.Run(current.name, func(t *testing.T) {
			handler := withStubbedProxy(t, current.openAIJSON)
			status, body := doRequest(t, handler)

			if status != http.StatusOK {
				t.Fatalf("status=%d want=%d body=%s", status, http.StatusOK, body)
			}
			if body != current.wantBody {
				t.Fatalf("got body %q want %q", body, current.wantBody)
			}
		})
	}
}

// Proves precedence is output > response > choices.
func Test_ResponsePrecedence_EndToEnd(t *testing.T) {
	compound := `{
		"choices":[{"message":{"content":"C"}}],
		"response":[{"content":[{"type":"output_text","text":"R"}]}],
		"output":[{"type":"output_text","text":"O"}]
	}`
	handler := withStubbedProxy(t, compound)
	status, body := doRequest(t, handler)

	if status != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", status, http.StatusOK, body)
	}
	if body != "O" {
		t.Fatalf("precedence wrong: got %q want %q", body, "O")
	}
}
