package proxy_test

import (
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
)

const messageUnexpectedPollTimeout = "upstreamPollTimeoutSeconds=%d want=%d"

// TestApplyTunablesSetsDefaultUpstreamPollTimeout verifies the default poll timeout is applied.
func TestApplyTunablesSetsDefaultUpstreamPollTimeout(testingInstance *testing.T) {
	configuration := proxy.Configuration{}
	configuration.ApplyTunables()
	if configuration.UpstreamPollTimeoutSeconds != proxy.DefaultUpstreamPollTimeoutSeconds {
		testingInstance.Fatalf(messageUnexpectedPollTimeout, configuration.UpstreamPollTimeoutSeconds, proxy.DefaultUpstreamPollTimeoutSeconds)
	}
}
