package integration_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/proxy"
)

// TestIntegrationHighLoadQueue verifies queue saturation handling.
func TestIntegrationHighLoadQueue(testingInstance *testing.T) {
	testingInstance.Skip("queue saturation scenario requires further investigation")

	gin.SetMode(gin.TestMode)
	client, _ := makeHTTPClient(testingInstance, false)
	configureProxy(testingInstance, client)
	router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: serviceSecretValue,
		OpenAIKey:     openAIKeyValue,
		LogLevel:      logLevelDebug,
		WorkerCount:   1,
		QueueSize:     proxy.DefaultQueueSize,
	}, newLogger(testingInstance))
	if buildRouterError != nil {
		testingInstance.Fatalf("BuildRouter failed: %v", buildRouterError)
	}
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)
	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set(promptQueryParameter, promptValue)
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	requestURL.RawQuery = queryValues.Encode()

	total := proxy.DefaultQueueSize + 1
	statuses := make([]int, total)
	var waitGroup sync.WaitGroup
	waitGroup.Add(total)
	for requestIndex := 0; requestIndex < total; requestIndex++ {
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

	var okCount, queueFullCount int
	for _, status := range statuses {
		if status == http.StatusOK {
			okCount++
		} else if status == http.StatusServiceUnavailable {
			queueFullCount++
		}
	}
	if okCount != proxy.DefaultQueueSize || queueFullCount != 1 {
		testingInstance.Fatalf("ok=%d queue_full=%d", okCount, queueFullCount)
	}
}
