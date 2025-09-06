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

func (roundTripper roundTripperFunc) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	return roundTripper(httpRequest)
}

// newRouterWithStubbedOpenAI returns a router that uses a stubbed OpenAI backend.
func newRouterWithStubbedOpenAI(testingInstance *testing.T, modelsBody, responsesBody string, workerCount, queueSize, requestTimeoutSeconds int) *gin.Engine {
	testingInstance.Helper()

	originalClient := proxy.HTTPClient
	testingInstance.Cleanup(func() { proxy.HTTPClient = originalClient })

	proxy.HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			switch request.URL.String() {
			case proxy.DefaultEndpoints.GetModelsURL():
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(modelsBody)),
					Header:     make(http.Header),
				}, nil
			case proxy.DefaultEndpoints.GetResponsesURL():
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
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:         "sekret",
		OpenAIKey:             "sk-test",
		LogLevel:              "debug",
		WorkerCount:           workerCount,
		QueueSize:             queueSize,
		RequestTimeoutSeconds: requestTimeoutSeconds,
	}, logger.Sugar())
	if buildError != nil {
		testingInstance.Fatalf("BuildRouter error: %v", buildError)
	}
	return router
}

// TestEndpoint_Empty200TreatedAsError ensures that empty successful responses are treated as errors.
func TestEndpoint_Empty200TreatedAsError(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)

	proxy.DefaultEndpoints.SetModelsURL("https://mock.local/v1/models")
	proxy.DefaultEndpoints.SetResponsesURL("https://mock.local/v1/responses")
	testingInstance.Cleanup(func() { proxy.DefaultEndpoints.ResetModelsURL() })
	testingInstance.Cleanup(func() { proxy.DefaultEndpoints.ResetResponsesURL() })

	router := newRouterWithStubbedOpenAI(
		testingInstance,
		`{"data":[{"id":"`+proxy.ModelNameGPT41+`"}]}`,
		`{"output":[]}`,
		1,
		4,
		5,
	)

	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	request, _ := http.NewRequest("GET", server.URL+"/?prompt=test&key=sekret", nil)
	response, requestError := http.DefaultClient.Do(request)
	if requestError != nil {
		testingInstance.Fatalf("request failed: %v", requestError)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusBadGateway {
		testingInstance.Fatalf("status=%d want=%d", response.StatusCode, http.StatusBadGateway)
	}
}

// TestEndpoint_RespectsAcceptHeaderCSV validates CSV responses when the Accept header requests text/csv.
func TestEndpoint_RespectsAcceptHeaderCSV(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)

	proxy.DefaultEndpoints.SetModelsURL("https://mock.local/v1/models")
	proxy.DefaultEndpoints.SetResponsesURL("https://mock.local/v1/responses")
	testingInstance.Cleanup(func() { proxy.DefaultEndpoints.ResetModelsURL() })
	testingInstance.Cleanup(func() { proxy.DefaultEndpoints.ResetResponsesURL() })

	router := newRouterWithStubbedOpenAI(
		testingInstance,
		`{"data":[{"id":"`+proxy.ModelNameGPT41+`"}]}`,
		`{"output_text":"Hello, world!"}`,
		1,
		4,
		5,
	)

	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	request, _ := http.NewRequest("GET", server.URL+"/?prompt=anything&key=sekret", nil)
	request.Header.Set("Accept", "text/csv")
	response, requestError := http.DefaultClient.Do(request)
	if requestError != nil {
		testingInstance.Fatalf("request failed: %v", requestError)
	}
	defer response.Body.Close()

	if contentType := response.Header.Get("Content-Type"); contentType != "text/csv" {
		testingInstance.Fatalf("content-type=%q want=%q", contentType, "text/csv")
	}
	responseBody, _ := io.ReadAll(response.Body)
	if body := string(responseBody); body != "\"Hello, world!\"\n" {
		testingInstance.Fatalf("body=%q want=%q", body, "\"Hello, world!\"\n")
	}
}

// TestEndpoint_ReturnsServiceUnavailableWhenQueueFull confirms that a full queue results in an HTTP 503 status code.
func TestEndpoint_ReturnsServiceUnavailableWhenQueueFull(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)

	proxy.DefaultEndpoints.SetModelsURL("https://mock.local/v1/models")
	proxy.DefaultEndpoints.SetResponsesURL("https://mock.local/v1/responses")
	testingInstance.Cleanup(func() { proxy.DefaultEndpoints.ResetModelsURL() })
	testingInstance.Cleanup(func() { proxy.DefaultEndpoints.ResetResponsesURL() })

	router := newRouterWithStubbedOpenAI(
		testingInstance,
		`{"data":[{"id":"`+proxy.ModelNameGPT41+`"}]}`,
		`{"output_text":"queued"}`,
		0,
		1,
		1,
	)

	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	firstRequest, _ := http.NewRequest("GET", server.URL+"/?prompt=first&key=sekret", nil)
	go http.DefaultClient.Do(firstRequest)
	time.Sleep(50 * time.Millisecond)

	secondRequest, _ := http.NewRequest("GET", server.URL+"/?prompt=second&key=sekret", nil)
	secondResponse, secondRequestError := http.DefaultClient.Do(secondRequest)
	if secondRequestError != nil {
		testingInstance.Fatalf("request failed: %v", secondRequestError)
	}
	defer secondResponse.Body.Close()

	if secondResponse.StatusCode != http.StatusServiceUnavailable {
		testingInstance.Fatalf("status=%d want=%d", secondResponse.StatusCode, http.StatusServiceUnavailable)
	}
	responseBody, _ := io.ReadAll(secondResponse.Body)
	const expectedBody = "request queue full"
	if strings.TrimSpace(string(responseBody)) != expectedBody {
		testingInstance.Fatalf("body=%q want=%q", string(responseBody), expectedBody)
	}
}
