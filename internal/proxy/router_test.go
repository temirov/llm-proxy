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

const (
	// finalResponse is the JSON payload returned by the session mock server.
	finalResponse = `{"status":"completed", "output":[{"type":"message", "role":"assistant", "content":[{"type":"text","text":"ok"}]}]}`
	// requestPathPattern formats the request path for the chat handler tests.
	requestPathPattern = "/?prompt=%s&model=%s&key=%s"
	// scenarioUnknownModelBadRequest describes the behavior when an unknown model is requested.
	scenarioUnknownModelBadRequest = "unknown model returns bad request"
	// scenarioKnownModelOK describes the behavior when a known model is requested.
	scenarioKnownModelOK = "known model returns ok"
	// unknownModelIdentifier is a model name not recognized by the proxy.
	unknownModelIdentifier = "unknown-model"
	// statusFormat formats the mismatch status code error message.
	statusFormat = "status=%d want=%d"
)

// TestChatHandlerValidatesModel verifies model validation and a successful request flow.
func TestChatHandlerValidatesModel(testingInstance *testing.T) {

	testScenarios := []chatHandlerScenario{
		{
			scenarioName:       scenarioUnknownModelBadRequest,
			modelIdentifier:    unknownModelIdentifier,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			scenarioName:       scenarioKnownModelOK,
			modelIdentifier:    proxy.ModelNameGPT4o,
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, testScenario := range testScenarios {
		testingInstance.Run(testScenario.scenarioName, func(subTestInstance *testing.T) {
			mockServer := NewSessionMockServer(finalResponse)
			defer mockServer.Close()
			router := NewTestRouter(subTestInstance, mockServer.URL)

			requestPath := fmt.Sprintf(requestPathPattern, TestPrompt, testScenario.modelIdentifier, TestSecret)
			request := httptest.NewRequest(http.MethodGet, requestPath, nil)
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != testScenario.expectedStatusCode {
				subTestInstance.Fatalf(statusFormat, responseRecorder.Code, testScenario.expectedStatusCode)
			}
		})
	}
}
