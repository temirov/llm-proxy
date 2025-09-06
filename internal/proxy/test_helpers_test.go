package proxy_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

// Test constants used across the entire test suite for this package.
const (
	TestJobID                    = "resp_test_12345"
	messageBuildRouterError      = "BuildRouter error: %v"
	messageUnexpectedPollTimeout = "upstreamPollTimeout=%v want=%v"
)

// NewSessionMockServer creates a reusable httptest.Server that correctly
// simulates the multi-step session flow for the Responses API.
func NewSessionMockServer(finalResponseJSON string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Handle the initial POST to create the session.
		if r.Method == http.MethodPost && r.URL.Path == "/" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"id": "%s", "status": "queued"}`, TestJobID)))
			return
		}
		// 2. Handle the subsequent GET to poll the session.
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, TestJobID) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(finalResponseJSON))
			return
		}
		// 3. Handle a "continue" POST if a test requires it.
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/continue") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status": "in_progress"}`)) // Acknowledge the continue request
			return
		}
		http.NotFound(w, r)
	}))
}

// NewTestRouter creates a pre-configured router for integration tests.
func NewTestRouter(t *testing.T, serverURL string) *gin.Engine {
	t.Helper()
	endpointConfiguration := proxy.NewEndpoints()
	endpointConfiguration.SetResponsesURL(serverURL)

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
		Endpoints:                  endpointConfiguration,
	}, logger.Sugar())
	if err != nil {
		t.Fatalf("BuildRouter error: %v", err)
	}
	return router
}
