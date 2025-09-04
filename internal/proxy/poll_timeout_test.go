package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/temirov/llm-proxy/internal/proxy"
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

	proxy.SetModelsURL(modelsServer.URL + modelsPath)
	proxy.HTTPClient = modelsServer.Client()
	testFramework.Cleanup(proxy.ResetModelsURL)
	testFramework.Cleanup(func() { proxy.HTTPClient = http.DefaultClient })

	loggerInstance, _ := zap.NewDevelopment()
	defer loggerInstance.Sync()

	previousPollTimeout := proxy.UpstreamPollTimeout()
	defer proxy.SetUpstreamPollTimeout(previousPollTimeout)

	_, buildRouterError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              serviceSecretValue,
		OpenAIKey:                  openAIKeyValue,
		LogLevel:                   proxy.LogLevelDebug,
		WorkerCount:                1,
		QueueSize:                  1,
		UpstreamPollTimeoutSeconds: 0,
	}, loggerInstance.Sugar())
	if buildRouterError != nil {
		testFramework.Fatalf(messageBuildRouterError, buildRouterError)
	}

	expectedDuration := time.Duration(proxy.DefaultUpstreamPollTimeoutSeconds) * time.Second
	if proxy.UpstreamPollTimeout() != expectedDuration {
		testFramework.Fatalf(messageUnexpectedPollTimeout, proxy.UpstreamPollTimeout(), expectedDuration)
	}
}
