package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/proxy"
)

const missingPromptErrorMessage = "missing prompt parameter"

// TestRequestWithoutPromptReturnsMissingPromptError ensures that a request lacking the prompt query parameter yields a 400 status with the missing prompt error message.
func TestRequestWithoutPromptReturnsMissingPromptError(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	endpoints := proxy.NewEndpoints()
	client, _ := makeHTTPClient(testingInstance, false, endpoints)
	configureProxy(testingInstance, client, endpoints)
	router, buildError := proxy.BuildRouter(proxy.Configuration{ServiceSecret: serviceSecretValue, OpenAIKey: openAIKeyValue, LogLevel: logLevelDebug, WorkerCount: 1, QueueSize: 8, Endpoints: endpoints}, newLogger(testingInstance))
	if buildError != nil {
		testingInstance.Fatalf("BuildRouter failed: %v", buildError)
	}
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)
	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	requestURL.RawQuery = queryValues.Encode()
	httpResponse, requestError := http.Get(requestURL.String())
	if requestError != nil {
		testingInstance.Fatalf("GET failed: %v", requestError)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusBadRequest {
		testingInstance.Fatalf("status=%d want=%d", httpResponse.StatusCode, http.StatusBadRequest)
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if string(responseBytes) != missingPromptErrorMessage {
		testingInstance.Fatalf("body=%q want=%q", string(responseBytes), missingPromptErrorMessage)
	}
}
