package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

// Constants shared across integration tests.
const (
	// serviceSecretValue is the service secret used for test requests.
	serviceSecretValue = "sekret"
	// openAIKeyValue is the OpenAI API key used in tests.
	openAIKeyValue = "sk-test"
	// logLevelDebug represents the debug logging level.
	logLevelDebug = "debug"
	// mockModelsURL is the stub URL for the models endpoint.
	mockModelsURL = "https://mock.local/v1/models"
	// mockResponsesURL is the stub URL for the responses endpoint.
	mockResponsesURL = "https://mock.local/v1/responses"
	// promptQueryParameter is the name of the prompt query string parameter.
	promptQueryParameter = "prompt"
	// keyQueryParameter is the name of the service secret query string parameter.
	keyQueryParameter = "key"
	// promptValue is the prompt used for test requests.
	promptValue = "ping"
	// integrationServiceSecret is the service secret for OpenAI stub server tests.
	integrationServiceSecret = serviceSecretValue
	// integrationOpenAIKey is the OpenAI API key for the stub server tests.
	integrationOpenAIKey = openAIKeyValue
	// integrationModelsPath is the path for the models endpoint.
	integrationModelsPath = "/v1/models"
	// integrationResponsesPath is the path for the responses endpoint.
	integrationResponsesPath = "/v1/responses"
	// integrationModelListBody is the JSON body returned for model listing.
	integrationModelListBody = `{"object":"list","data":[{"id":"` + proxy.ModelNameGPT41 + `","object":"model"}]}`
	// integrationOKBody is the plain response used in tests.
	integrationOKBody = "INTEGRATION_OK"
	// integrationSearchBody is the web search response used in tests.
	integrationSearchBody = "SEARCH_OK"
	// availableModelsBody is the JSON body returned by the stubbed models endpoint in HTTP client tests.
	availableModelsBody = `{"data":[{"id":"` + proxy.ModelNameGPT41 + `"},{"id":"` + proxy.ModelNameGPT5Mini + `"},{"id":"` + proxy.ModelNameGPT5 + `"}]}`
	// contentTypeJSON is the HTTP header value for JSON payloads.
	contentTypeJSON = "application/json"
	// buildRouterErrorFormat is the format string used when BuildRouter returns an error.
	buildRouterErrorFormat = "BuildRouter error: %v"
	// buildRouterFailedFormat is the format string used when BuildRouter fails in tests.
	buildRouterFailedFormat = "BuildRouter failed: %v"
	// unexpectedRequestFormat is the format string used when an unexpected request occurs.
	unexpectedRequestFormat = "unexpected request to %s"
	// requestErrorFormat is the format string used when a request fails.
	requestErrorFormat = "request error: %v"
	// unexpectedStatusFormat is the format string used when an unexpected HTTP status is returned.
	unexpectedStatusFormat = "status=%d body=%s"
	// bodyMismatchFormat reports differing response bodies.
	bodyMismatchFormat = "body=%q want=%q"
	// toolsMissingMessage reports missing tools in captured payloads.
	toolsMissingMessage = "tools missing when web_search=1"
	// toolsMissingFormat reports missing tools when the captured payload is included.
	toolsMissingFormat = "tools missing in payload when web_search=1; captured=%v"
	// toolsOmittedFormat reports an unexpected tools field for models without tool support.
	toolsOmittedFormat = "tools must be omitted for %s, got: %v"
	// temperatureOmittedFormat reports a temperature field when it should be omitted.
	temperatureOmittedFormat = "temperature must be omitted for %s, got: %v"
	// toolTypeMismatchFormat reports an unexpected tool type.
	toolTypeMismatchFormat = "tool type=%v want=web_search"
	// metadataTemperatureTools provides model metadata allowing temperature and tools.
	metadataTemperatureTools = `{"allowed_request_fields":["temperature","tools"]}`
	// metadataEmpty provides model metadata with no allowed request fields.
	metadataEmpty = `{"allowed_request_fields":[]}`
	// expectedErrorFormat is used when a configuration error is expected.
	expectedErrorFormat = "expected %s error, got %v"
	// getFailedFormat reports HTTP GET request failures.
	getFailedFormat = "GET failed: %v"
	// statusWantFormat reports an unexpected HTTP status code.
	statusWantFormat = "status=%d want=%d"
	// statusWantBodyFormat reports unexpected status codes and response bodies.
	statusWantBodyFormat = "status=%d want=%d body=%q"
)

// roundTripperFunc allows custom RoundTrip implementations for the HTTP client.
type roundTripperFunc func(httpRequest *http.Request) (*http.Response, error)

// RoundTrip executes the custom RoundTrip implementation.
func (roundTripper roundTripperFunc) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	return roundTripper(httpRequest)
}

// newOpenAIServer returns a stub OpenAI server yielding the provided body and optionally capturing requests.
func newOpenAIServer(testingInstance *testing.T, responseText string, captureTarget *any) *httptest.Server {
	testingInstance.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch httpRequest.URL.Path {
		case integrationModelsPath:
			responseWriter.Header().Set("Content-Type", contentTypeJSON)
			_, _ = io.WriteString(responseWriter, integrationModelListBody)
		case integrationResponsesPath:
			if captureTarget != nil {
				requestBytes, _ := io.ReadAll(httpRequest.Body)
				_ = json.Unmarshal(requestBytes, captureTarget)
			}
			responseWriter.Header().Set("Content-Type", contentTypeJSON)
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
	endpointConfiguration := proxy.NewEndpoints()
	endpointConfiguration.SetModelsURL(openAIServer.URL + integrationModelsPath)
	endpointConfiguration.SetResponsesURL(openAIServer.URL + integrationResponsesPath)
	proxy.HTTPClient = openAIServer.Client()
	testingInstance.Cleanup(func() { proxy.HTTPClient = http.DefaultClient })
	loggerInstance, _ := zap.NewDevelopment()
	testingInstance.Cleanup(func() { _ = loggerInstance.Sync() })
	router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: integrationServiceSecret,
		OpenAIKey:     integrationOpenAIKey,
		LogLevel:      logLevelDebug,
		WorkerCount:   1,
		QueueSize:     4,
		Endpoints:     endpointConfiguration,
	}, loggerInstance.Sugar())
	if buildRouterError != nil {
		testingInstance.Fatalf(buildRouterErrorFormat, buildRouterError)
	}
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)
	return server
}

// makeHTTPClient returns a stub HTTP client capturing payloads and returning canned responses.
func makeHTTPClient(testingInstance *testing.T, wantWebSearch bool) (*http.Client, *map[string]any) {
	testingInstance.Helper()
	var captured map[string]any
	return &http.Client{
		Transport: roundTripperFunc(func(httpRequest *http.Request) (*http.Response, error) {
			switch {
			case httpRequest.URL.String() == mockModelsURL:
				body := availableModelsBody
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			case strings.HasPrefix(httpRequest.URL.String(), mockModelsURL+"/"):
				modelID := strings.TrimPrefix(httpRequest.URL.Path, integrationModelsPath+"/")
				metadata := metadataEmpty
				if modelID == proxy.ModelNameGPT41 {
					metadata = metadataTemperatureTools
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(metadata)), Header: make(http.Header)}, nil
			case httpRequest.URL.String() == mockResponsesURL:
				if httpRequest.Body != nil {
					requestBytes, _ := io.ReadAll(httpRequest.Body)
					_ = json.Unmarshal(requestBytes, &captured)
				}
				text := integrationOKBody
				if wantWebSearch {
					text = integrationSearchBody
				}
				body := `{"output_text":"` + text + `"}`
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			default:
				testingInstance.Fatalf(unexpectedRequestFormat, httpRequest.URL.String())
				return nil, nil
			}
		}),
		Timeout: 5 * time.Second,
	}, &captured
}

// newLogger constructs a development logger for tests.
func newLogger(testingInstance *testing.T) *zap.SugaredLogger {
	testingInstance.Helper()
	loggerInstance, _ := zap.NewDevelopment()
	testingInstance.Cleanup(func() { _ = loggerInstance.Sync() })
	return loggerInstance.Sugar()
}

// configureProxy sets URLs and the HTTP client for proxy operations.
func configureProxy(testingInstance *testing.T, client *http.Client) *proxy.Endpoints {
	testingInstance.Helper()
	proxy.HTTPClient = client
	testingInstance.Cleanup(func() { proxy.HTTPClient = http.DefaultClient })
	endpointConfiguration := proxy.NewEndpoints()
	endpointConfiguration.SetModelsURL(mockModelsURL)
	endpointConfiguration.SetResponsesURL(mockResponsesURL)
	return endpointConfiguration
}
