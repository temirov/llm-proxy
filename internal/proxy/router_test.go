package proxy_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
)

// chatHandlerScenario defines a single test scenario for model validation.
type chatHandlerScenario struct {
	scenarioName       string
	modelIdentifier    string
	expectedStatusCode int
}

// TestChatHandlerValidatesModel verifies model validation and a successful request flow.
func TestChatHandlerValidatesModel(t *testing.T) {
	// Corrected to use "text" to match the parser's expectation.
	const finalResponse = `{"status":"completed", "output":[{"type":"message", "role":"assistant", "content":[{"type":"text","text":"ok"}]}]}`

	testScenarios := []chatHandlerScenario{
		{
			scenarioName:       "unknown model returns bad request",
			modelIdentifier:    "unknown-model",
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			scenarioName:       "known model returns ok",
			modelIdentifier:    proxy.ModelNameGPT4o,
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, testScenario := range testScenarios {
		t.Run(testScenario.scenarioName, func(t *testing.T) {
			mockServer := NewSessionMockServer(finalResponse)
			defer mockServer.Close()
			router := NewTestRouter(t, mockServer.URL)

			requestPath := fmt.Sprintf("/?prompt=%s&model=%s&key=%s", TestPrompt, testScenario.modelIdentifier, TestSecret)
			request := httptest.NewRequest(http.MethodGet, requestPath, nil)
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != testScenario.expectedStatusCode {
				t.Fatalf("status=%d want=%d", responseRecorder.Code, testScenario.expectedStatusCode)
			}
		})
	}
}
