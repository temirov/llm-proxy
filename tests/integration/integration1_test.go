package integration_test

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
	// toolsField identifies the tools request field.
	toolsField = "tools"
	// toolChoiceField identifies the tool_choice request field.
	toolChoiceField = "tool_choice"
	// reasoningField identifies the reasoning request field.
	reasoningField = "reasoning"
	// effortField identifies the effort field within the reasoning object.
	effortField = "effort"
	// typeField identifies the type field within a tool descriptor.
	typeField = "type"
	// toolChoiceAutoValue is the expected value of the tool_choice field when web search is enabled.
	toolChoiceAutoValue = "auto"
	// reasoningEffortMediumValue is the expected reasoning effort for GPT-5.
	reasoningEffortMediumValue = "medium"
	// toolTypeWebSearchValue is the expected tool type when web search is requested.
	toolTypeWebSearchValue = "web_search"
	// toolChoiceMismatchFormat reports an unexpected tool_choice value.
	toolChoiceMismatchFormat = "tool_choice=%v want=%v"
	// reasoningMissingFormat reports a missing reasoning field in the payload.
	reasoningMissingFormat = "reasoning missing in payload: %v"
	// reasoningEffortMismatchFormat reports an unexpected reasoning effort value.
	reasoningEffortMismatchFormat = "reasoning effort=%v want=%v"
)

// newIntegrationServerWithTimeout builds the application server pointing at the stub OpenAI server with a configurable request timeout.
func newIntegrationServerWithTimeout(testingInstance *testing.T, openAIServer *httptest.Server, requestTimeoutSeconds int) *httptest.Server {
	testingInstance.Helper()
	endpoints := proxy.NewEndpoints()
	endpoints.SetModelsURL(openAIServer.URL + integrationModelsPath)
	endpoints.SetResponsesURL(openAIServer.URL + integrationResponsesPath)
	originalClient := proxy.HTTPClient
	proxy.HTTPClient = openAIServer.Client()
	testingInstance.Cleanup(func() { proxy.HTTPClient = originalClient })
	logger, _ := zap.NewDevelopment()
	testingInstance.Cleanup(func() { _ = logger.Sync() })
	router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:         integrationServiceSecret,
		OpenAIKey:             integrationOpenAIKey,
		LogLevel:              logLevelDebug,
		WorkerCount:           1,
		QueueSize:             4,
		RequestTimeoutSeconds: requestTimeoutSeconds,
		Endpoints:             endpoints,
	}, logger.Sugar())
	if buildRouterError != nil {
		testingInstance.Fatalf(buildRouterErrorFormat, buildRouterError)
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
				subTest.Fatalf(requestErrorFormat, requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != http.StatusOK {
				responseBody, _ := io.ReadAll(httpResponse.Body)
				subTest.Fatalf(unexpectedStatusFormat, httpResponse.StatusCode, string(responseBody))
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if string(responseBytes) != testCase.body {
				subTest.Fatalf(bodyMismatchFormat, string(responseBytes), testCase.body)
			}
			if testCase.checkTools {
				capturedMap, _ := captured.(map[string]any)
				tools, ok := capturedMap["tools"].([]any)
				if !ok || len(tools) == 0 {
					subTest.Fatal(toolsMissingMessage)
				}
				first, _ := tools[0].(map[string]any)
				if first["type"] != "web_search" {
					subTest.Fatalf(toolTypeMismatchFormat, first["type"])
				}
			}
		})
	}
}

// TestProxyGPT5WebSearchIncludesReasoning verifies that GPT-5 requests with web search
// include tools, tool_choice, and reasoning fields.
func TestProxyGPT5WebSearchIncludesReasoning(testingInstance *testing.T) {
	var capturedPayload any
	openAIServer := newOpenAIServer(testingInstance, integrationSearchBody, &capturedPayload)
	testingInstance.Cleanup(openAIServer.Close)
	applicationServer := newIntegrationServer(testingInstance, openAIServer)
	requestURL, _ := url.Parse(applicationServer.URL)
	queryValues := requestURL.Query()
	queryValues.Set(promptQueryParameter, promptValue)
	queryValues.Set(keyQueryParameter, integrationServiceSecret)
	queryValues.Set(webSearchQueryParameter, "1")
	queryValues.Set(adaptiveModelQueryParameter, proxy.ModelNameGPT5)
	requestURL.RawQuery = queryValues.Encode()
	httpResponse, requestError := http.Get(requestURL.String())
	if requestError != nil {
		testingInstance.Fatalf(requestErrorFormat, requestError)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		testingInstance.Fatalf(unexpectedStatusFormat, httpResponse.StatusCode, string(responseBody))
	}
	_, _ = io.ReadAll(httpResponse.Body)
	payloadMap, _ := capturedPayload.(map[string]any)
	toolsValue, ok := payloadMap[toolsField].([]any)
	if !ok || len(toolsValue) == 0 {
		testingInstance.Fatalf(toolsMissingFormat, payloadMap)
	}
	firstTool, _ := toolsValue[0].(map[string]any)
	if firstTool[typeField] != toolTypeWebSearchValue {
		testingInstance.Fatalf(toolTypeMismatchFormat, firstTool[typeField])
	}
	toolChoiceValue, ok := payloadMap[toolChoiceField].(string)
	if !ok || toolChoiceValue != toolChoiceAutoValue {
		testingInstance.Fatalf(toolChoiceMismatchFormat, payloadMap[toolChoiceField], toolChoiceAutoValue)
	}
	reasoningValue, ok := payloadMap[reasoningField].(map[string]any)
	if !ok {
		testingInstance.Fatalf(reasoningMissingFormat, payloadMap)
	}
	effortValue, ok := reasoningValue[effortField].(string)
	if !ok || effortValue != reasoningEffortMediumValue {
		testingInstance.Fatalf(reasoningEffortMismatchFormat, reasoningValue[effortField], reasoningEffortMediumValue)
	}
}
