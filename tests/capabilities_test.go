package tests_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

// Constants used in tests.
const (
	modelIDGPT4o                   = proxy.ModelNameGPT4o
	modelIDGPT4oMini               = proxy.ModelNameGPT4oMini
	modelIDGPT5Mini                = proxy.ModelNameGPT5Mini
	serviceSecret                  = "sekret"
	openAIKey                      = "sk-test"
	logLevel                       = "debug"
	openAIResponsesPath            = "/v1/responses"
	openAIResponseTemplate         = `{"output_text":"%s"}`
	responseTextWithoutTools       = "NO_TOOLS_OK"
	responseTextWithoutTemperature = "TEMPLESS_OK"
)

// TestIntegration_OmitsDisallowedParameters confirms that metadata disallowed fields are removed from requests.
func TestIntegration_OmitsDisallowedParameters(testingInstance *testing.T) {
	testCases := []struct {
		testName         string
		modelIdentifier  string
		additionalQuery  string
		expectedResponse string
		disallowedFields []string
	}{
		{
			testName:         "temperature omitted",
			modelIdentifier:  modelIDGPT5Mini,
			additionalQuery:  "",
			expectedResponse: responseTextWithoutTemperature,
			disallowedFields: []string{"temperature"},
		},
		{
			testName:         "tools omitted",
			modelIdentifier:  modelIDGPT4oMini,
			additionalQuery:  "&web_search=1",
			expectedResponse: responseTextWithoutTools,
			disallowedFields: []string{"tools", "tool_choice"},
		},
	}

	for _, testCase := range testCases {
		currentTestCase := testCase
		testingInstance.Run(currentTestCase.testName, func(subTestInstance *testing.T) {
			var observed any

			openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
				switch {
				case strings.HasSuffix(httpRequest.URL.Path, openAIResponsesPath):
					body, _ := io.ReadAll(httpRequest.Body)
					_ = json.Unmarshal(body, &observed)
					io.WriteString(responseWriter, fmt.Sprintf(openAIResponseTemplate, currentTestCase.expectedResponse))
				default:
					http.NotFound(responseWriter, httpRequest)
				}
			}))
			defer openAIServer.Close()

			endpointConfiguration := proxy.NewEndpoints()
			endpointConfiguration.SetResponsesURL(openAIServer.URL + openAIResponsesPath)
			proxy.HTTPClient = openAIServer.Client()
			subTestInstance.Cleanup(func() { proxy.HTTPClient = http.DefaultClient })

			logger, _ := zap.NewDevelopment()
			defer logger.Sync()

			router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
				ServiceSecret: serviceSecret,
				OpenAIKey:     openAIKey,
				LogLevel:      logLevel,
				WorkerCount:   1,
				QueueSize:     4,
				Endpoints:     endpointConfiguration,
			}, logger.Sugar())
			if buildRouterError != nil {
				subTestInstance.Fatalf("BuildRouter error: %v", buildRouterError)
			}

			applicationServer := httptest.NewServer(router)
			defer applicationServer.Close()

			httpResponse, requestError := http.Get(applicationServer.URL + "/?prompt=hello&key=" + serviceSecret + "&model=" + currentTestCase.modelIdentifier + currentTestCase.additionalQuery)
			if requestError != nil {
				subTestInstance.Fatalf("request failed: %v", requestError)
			}
			defer httpResponse.Body.Close()

			if httpResponse.StatusCode != http.StatusOK {
				responseBody, _ := io.ReadAll(httpResponse.Body)
				subTestInstance.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBody))
			}
			if payload, ok := observed.(map[string]any); ok {
				for _, fieldName := range currentTestCase.disallowedFields {
					if _, found := payload[fieldName]; found {
						subTestInstance.Fatalf("%s present in payload: %v", fieldName, payload)
					}
				}
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if strings.TrimSpace(string(responseBytes)) != currentTestCase.expectedResponse {
				subTestInstance.Fatalf("body=%q want %q", string(responseBytes), currentTestCase.expectedResponse)
			}
		})
	}
}
