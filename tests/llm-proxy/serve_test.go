package llm_proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

type roundTripperFunc func(httpRequest *http.Request) (*http.Response, error)

func (transport roundTripperFunc) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	return transport(httpRequest)
}

// newRouterWithStubbedOpenAI returns a router that uses a stubbed OpenAI backend.
func newRouterWithStubbedOpenAI(testingContext *testing.T, modelsBody, responsesBody string) *gin.Engine {
	testingContext.Helper()

	orig := proxy.HTTPClient
	testingContext.Cleanup(func() { proxy.HTTPClient = orig })

	proxy.HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			switch request.URL.String() {
			case proxy.ModelsURL():
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(modelsBody)),
					Header:     make(http.Header),
				}, nil
			case proxy.ResponsesURL():
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(responsesBody)),
					Header:     make(http.Header),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`{}`)),
					Header:     make(http.Header),
				}, nil
			}
		}),
		Timeout: 5 * time.Second,
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     4,
	}, logger.Sugar())
	if err != nil {
		testingContext.Fatalf("BuildRouter error: %v", err)
	}
	return router
}

func TestEndpoint_Empty200TreatedAsError(testingContext *testing.T) {
	gin.SetMode(gin.TestMode)

	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	testingContext.Cleanup(proxy.ResetModelsURL)
	testingContext.Cleanup(proxy.ResetResponsesURL)

	router := newRouterWithStubbedOpenAI(
		testingContext,
		`{"data":[{"id":"gpt-4.1"}]}`,
		`{"output":[]}`,
	)

	testServer := httptest.NewServer(router)
	testingContext.Cleanup(testServer.Close)

	httpRequest, _ := http.NewRequest("GET", testServer.URL+"/?prompt=test&key=sekret", nil)
	httpResponse, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		testingContext.Fatalf("request failed: %v", err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusBadGateway {
		testingContext.Fatalf("status=%d want=%d", httpResponse.StatusCode, http.StatusBadGateway)
	}
}

func TestEndpoint_RespectsAcceptHeaderCSV(testingContext *testing.T) {
	gin.SetMode(gin.TestMode)

	proxy.SetModelsURL("https://mock.local/v1/models")
	proxy.SetResponsesURL("https://mock.local/v1/responses")
	testingContext.Cleanup(proxy.ResetModelsURL)
	testingContext.Cleanup(proxy.ResetResponsesURL)

	router := newRouterWithStubbedOpenAI(
		testingContext,
		`{"data":[{"id":"gpt-4.1"}]}`,
		`{"output_text":"Hello, world!"}`,
	)

	testServer := httptest.NewServer(router)
	testingContext.Cleanup(testServer.Close)

	httpRequest, _ := http.NewRequest("GET", testServer.URL+"/?prompt=anything&key=sekret", nil)
	httpRequest.Header.Set("Accept", "text/csv")
	httpResponse, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		testingContext.Fatalf("request failed: %v", err)
	}
	defer httpResponse.Body.Close()

	contentType := httpResponse.Header.Get("Content-Type")
	if contentType != "text/csv" {
		testingContext.Fatalf("content-type=%q want=%q", contentType, "text/csv")
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if bodyText := string(responseBytes); bodyText != "\"Hello, world!\"\n" {
		testingContext.Fatalf("body=%q want=%q", bodyText, "\"Hello, world!\"\n")
	}
}
