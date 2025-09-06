package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/proxy"
)

const (
	// queueRequestDelay is the time each upstream call sleeps to keep workers busy.
	queueRequestDelay = 2 * time.Second
	// requestTimeoutSeconds is the proxy request timeout used for queue saturation.
	requestTimeoutSeconds = 1
	// singleWorkerCount specifies the number of workers used in this test.
	singleWorkerCount = 1
	// queueFullCountFormat reports the number of queue-full responses.
	queueFullCountFormat = "queue_full=%d"
)

// makeDelayedHTTPClient returns an HTTP client that delays responses to simulate a slow upstream server.
func makeDelayedHTTPClient(testingInstance *testing.T, endpoints *proxy.Endpoints) *http.Client {
	testingInstance.Helper()
	return &http.Client{Transport: roundTripperFunc(func(httpRequest *http.Request) (*http.Response, error) {
		switch {
		case httpRequest.URL.String() == endpoints.GetModelsURL():
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(availableModelsBody)), Header: make(http.Header)}, nil
		case strings.HasPrefix(httpRequest.URL.String(), endpoints.GetModelsURL()+"/"):
			modelIdentifier := strings.TrimPrefix(httpRequest.URL.Path, integrationModelsPath+"/")
			metadata := metadataEmpty
			if modelIdentifier == proxy.ModelNameGPT41 {
				metadata = metadataTemperatureTools
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(metadata)), Header: make(http.Header)}, nil
		case httpRequest.URL.String() == endpoints.GetResponsesURL():
			time.Sleep(queueRequestDelay)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"output_text":"` + integrationOKBody + `"}`)), Header: make(http.Header)}, nil
		default:
			testingInstance.Fatalf(unexpectedRequestFormat, httpRequest.URL.String())
			return nil, nil
		}
	})}
}

// TestIntegrationHighLoadQueue verifies queue saturation handling.
func TestIntegrationHighLoadQueue(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	endpoints := proxy.NewEndpoints()
	client := makeDelayedHTTPClient(testingInstance, endpoints)
	configureProxy(testingInstance, client, endpoints)
	router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:         serviceSecretValue,
		OpenAIKey:             openAIKeyValue,
		LogLevel:              logLevelDebug,
		WorkerCount:           singleWorkerCount,
		QueueSize:             proxy.DefaultQueueSize,
		RequestTimeoutSeconds: requestTimeoutSeconds,
		Endpoints:             endpoints,
	}, newLogger(testingInstance))
	if buildRouterError != nil {
		testingInstance.Fatalf(buildRouterFailedFormat, buildRouterError)
	}
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)
	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set(promptQueryParameter, promptValue)
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	requestURL.RawQuery = queryValues.Encode()

	totalRequests := proxy.DefaultQueueSize + singleWorkerCount + 1
	statuses := make([]int, totalRequests)
	var waitGroup sync.WaitGroup
	waitGroup.Add(totalRequests)
	for requestIndex := 0; requestIndex < totalRequests; requestIndex++ {
		go func(index int) {
			defer waitGroup.Done()
			httpResponse, requestError := http.Get(requestURL.String())
			if requestError != nil {
				return
			}
			statuses[index] = httpResponse.StatusCode
			httpResponse.Body.Close()
		}(requestIndex)
	}
	waitGroup.Wait()

	var queueFullCount int
	for _, status := range statuses {
		if status == http.StatusServiceUnavailable {
			queueFullCount++
		}
	}
	if queueFullCount != 1 {
		testingInstance.Fatalf(queueFullCountFormat, queueFullCount)
	}
}
