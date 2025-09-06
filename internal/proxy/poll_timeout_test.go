package proxy_test

import (
	"testing"
	"time"

	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

// TestBuildRouterAppliesDefaultUpstreamPollTimeout verifies that BuildRouter sets the
// upstream poll timeout to the default value when the configuration omits it.
func TestBuildRouterAppliesDefaultUpstreamPollTimeout(testFramework *testing.T) {
	loggerInstance, _ := zap.NewDevelopment()
	defer func() { _ = loggerInstance.Sync() }()

	previousPollTimeout := proxy.UpstreamPollTimeout()
	defer proxy.SetUpstreamPollTimeout(previousPollTimeout)

	_, buildRouterError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		LogLevel:                   proxy.LogLevelDebug,
		WorkerCount:                1,
		QueueSize:                  1,
		UpstreamPollTimeoutSeconds: 0, // Explicitly set to 0 to test default behavior
	}, loggerInstance.Sugar())

	if buildRouterError != nil {
		testFramework.Fatalf(messageBuildRouterError, buildRouterError)
	}

	expectedDuration := time.Duration(proxy.DefaultUpstreamPollTimeoutSeconds) * time.Second
	if proxy.UpstreamPollTimeout() != expectedDuration {
		testFramework.Fatalf(messageUnexpectedPollTimeout, proxy.UpstreamPollTimeout(), expectedDuration)
	}
}
