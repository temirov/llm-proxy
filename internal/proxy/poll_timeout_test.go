package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

// Test string constants.
const (
	serviceSecretValue           = "sekret"
	openAIKeyValue               = "sk-test"
	modelsPath                   = "/v1/models"
	modelsListResponse           = "{\"data\":[{\"id\":\"gpt-4o\"}]}"
	messageBuildRouterError      = "BuildRouter error: %v"
	messageUnexpectedPollTimeout = "upstreamPollTimeout=%v want=%v"
)

// TestBuildRouterAppliesDefaultUpstreamPollTimeout verifies that BuildRouter sets the upstream poll timeout to the default value when the configuration omits UpstreamPollTimeoutSeconds.
func TestBuildRouterAppliesDefaultUpstreamPollTimeout(testFramework *testing.T) {
	modelsServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		io.WriteString(responseWriter, modelsListResponse)
	}))
	defer modelsServer.Close()

	SetModelsURL(modelsServer.URL + modelsPath)
	HTTPClient = modelsServer.Client()
	testFramework.Cleanup(ResetModelsURL)
	testFramework.Cleanup(func() { HTTPClient = http.DefaultClient })

	loggerInstance, _ := zap.NewDevelopment()
	defer loggerInstance.Sync()

	previousPollTimeout := upstreamPollTimeout
	defer func() { upstreamPollTimeout = previousPollTimeout }()

	_, buildRouterError := BuildRouter(Configuration{
		ServiceSecret:              serviceSecretValue,
		OpenAIKey:                  openAIKeyValue,
		LogLevel:                   LogLevelDebug,
		WorkerCount:                1,
		QueueSize:                  1,
		UpstreamPollTimeoutSeconds: 0,
	}, loggerInstance.Sugar())
	if buildRouterError != nil {
		testFramework.Fatalf(messageBuildRouterError, buildRouterError)
	}

	expectedDuration := time.Duration(DefaultUpstreamPollTimeoutSeconds) * time.Second
	if upstreamPollTimeout != expectedDuration {
		testFramework.Fatalf(messageUnexpectedPollTimeout, upstreamPollTimeout, expectedDuration)
	}
}
