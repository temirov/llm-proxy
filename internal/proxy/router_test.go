package proxy_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

const (
	// promptValue holds the prompt sent in requests.
	promptValue = "hello"
	// knownModelValue identifies a valid model recognized by the validator.
	knownModelValue = proxy.ModelNameGPT4o
	// unknownModelValue identifies a model absent from the validator.
	unknownModelValue = "unknown-model"
	// systemPromptValue provides the system prompt used by the router.
	systemPromptValue = "system"
	// routerServiceSecret is the expected client key.
	routerServiceSecret = "sekret"
	// routerOpenAIKey is a stub API key.
	routerOpenAIKey = "sk-test"
	// responsesBodyJSON is the canned response returned by the responses API.
	responsesBodyJSON = "{\"output\":[{\"content\":[{\"text\":\"ok\"}]}]}"
	// requestPathTemplate formats the request path with prompt, model, and key.
	requestPathTemplate = "/?prompt=%s&model=%s&key=%s"
	// errorFormatBuildRouter formats BuildRouter errors.
	errorFormatBuildRouter = "BuildRouter error: %v"
	// errorFormatUnexpectedStatus formats unexpected HTTP status errors.
	errorFormatUnexpectedStatus = "status=%d want=%d"
)

// chatHandlerScenario defines a single test scenario for model validation.
type chatHandlerScenario struct {
	scenarioName       string
	modelIdentifier    string
	expectedStatusCode int
}

// TestChatHandlerValidatesModel verifies that the chat handler returns appropriate status codes for valid and invalid model identifiers.
func TestChatHandlerValidatesModel(testingInstance *testing.T) {
	testScenarios := []chatHandlerScenario{
		{
			scenarioName:       "unknown model returns bad request",
			modelIdentifier:    unknownModelValue,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			scenarioName:       "known model returns ok",
			modelIdentifier:    knownModelValue,
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, currentScenario := range testScenarios {
		testingInstance.Run(currentScenario.scenarioName, func(subTest *testing.T) {
			responsesServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
				io.WriteString(responseWriter, responsesBodyJSON)
			}))
			subTest.Cleanup(responsesServer.Close)

			proxy.SetResponsesURL(responsesServer.URL)
			proxy.HTTPClient = http.DefaultClient
			subTest.Cleanup(proxy.ResetResponsesURL)
			subTest.Cleanup(func() { proxy.HTTPClient = http.DefaultClient })

			loggerInstance, _ := zap.NewDevelopment()
			defer loggerInstance.Sync()

			builtRouter, buildRouterError := proxy.BuildRouter(proxy.Configuration{
				ServiceSecret: routerServiceSecret,
				OpenAIKey:     routerOpenAIKey,
				LogLevel:      proxy.LogLevelDebug,
				SystemPrompt:  systemPromptValue,
				WorkerCount:   1,
				QueueSize:     1,
			}, loggerInstance.Sugar())
			if buildRouterError != nil {
				subTest.Fatalf(errorFormatBuildRouter, buildRouterError)
			}

			responseRecorder := httptest.NewRecorder()
			requestPath := fmt.Sprintf(requestPathTemplate, promptValue, currentScenario.modelIdentifier, routerServiceSecret)
			request := httptest.NewRequest(http.MethodGet, requestPath, nil)

			builtRouter.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != currentScenario.expectedStatusCode {
				subTest.Fatalf(errorFormatUnexpectedStatus, responseRecorder.Code, currentScenario.expectedStatusCode)
			}
		})
	}
}
