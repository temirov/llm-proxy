package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

const (
	integrationServiceSecret = "sekret"
	integrationOpenAIKey     = "sk-test"
	integrationModelsPath    = "/v1/models"
	integrationResponsesPath = "/v1/responses"
	integrationModelListBody = `{"object":"list","data":[{"id":"gpt-4.1","object":"model"}]}`
	integrationOKBody        = "INTEGRATION_OK"
	integrationSearchBody    = "SEARCH_OK"
)

// newOpenAIServer returns a stub OpenAI server yielding the provided body and optionally capturing requests.
func newOpenAIServer(testingInstance *testing.T, responseText string, captureTarget *any) *httptest.Server {
	testingInstance.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch httpRequest.URL.Path {
		case integrationModelsPath:
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(responseWriter, integrationModelListBody)
		case integrationResponsesPath:
			if captureTarget != nil {
				body, _ := io.ReadAll(httpRequest.Body)
				_ = json.Unmarshal(body, captureTarget)
			}
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(responseWriter, `{"output_text":"`+responseText+`"}`)
		default:
			http.NotFound(responseWriter, httpRequest)
		}
	}))
	return server
}

// newIntegrationServer builds the application server pointing at the stub OpenAI server.
func newIntegrationServer(testingInstance *testing.T, openAIServer *httptest.Server) *httptest.Server {
	testingInstance.Helper()
	proxy.SetModelsURL(openAIServer.URL + integrationModelsPath)
	proxy.SetResponsesURL(openAIServer.URL + integrationResponsesPath)
	proxy.HTTPClient = openAIServer.Client()
	testingInstance.Cleanup(proxy.ResetModelsURL)
	testingInstance.Cleanup(proxy.ResetResponsesURL)
	logger, _ := zap.NewDevelopment()
	testingInstance.Cleanup(func() { _ = logger.Sync() })
	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: integrationServiceSecret,
		OpenAIKey:     integrationOpenAIKey,
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     4,
	}, logger.Sugar())
	if err != nil {
		testingInstance.Fatalf("BuildRouter error: %v", err)
	}
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)
	return server
}

// TestProxyResponseDelivery verifies responses with and without web search.
func TestProxyResponseDelivery(testingInstance *testing.T) {
	testCases := []struct {
		name       string
		webSearch  bool
		body       string
		checkTools bool
	}{
		{name: "plain", webSearch: false, body: integrationOKBody},
		{name: "web_search", webSearch: true, body: integrationSearchBody, checkTools: true},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			var captured any
			var captureTarget *any
			if testCase.checkTools {
				captureTarget = &captured
			}
			openAIServer := newOpenAIServer(subTest, testCase.body, captureTarget)
			subTest.Cleanup(openAIServer.Close)
			applicationServer := newIntegrationServer(subTest, openAIServer)
			requestURL := applicationServer.URL + "?prompt=ping&key=" + integrationServiceSecret
			if testCase.webSearch {
				requestURL += "&web_search=1"
			}
			httpResponse, requestError := http.Get(requestURL)
			if requestError != nil {
				subTest.Fatalf("request error: %v", requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != http.StatusOK {
				responseBody, _ := io.ReadAll(httpResponse.Body)
				subTest.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBody))
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if string(responseBytes) != testCase.body {
				subTest.Fatalf("body=%q want=%q", string(responseBytes), testCase.body)
			}
			if testCase.checkTools {
				mapped, _ := captured.(map[string]any)
				tools, ok := mapped["tools"].([]any)
				if !ok || len(tools) == 0 {
					subTest.Fatalf("tools missing when web_search=1")
				}
				first, _ := tools[0].(map[string]any)
				if first["type"] != "web_search" {
					subTest.Fatalf("tool type=%v want=web_search", first["type"])
				}
			}
		})
	}
}
