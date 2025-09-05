package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/proxy"
)

const (
	unsupportedModelIdentifier = "gpt-4o-mini"
	unsupportedErrorMessage    = "web_search is not supported by the selected model"
	modelQueryParameter        = "model"
)

// makeWebSearchRejectingHTTPClient returns an HTTP client whose model list includes only a model without web search support.
func makeWebSearchRejectingHTTPClient(testingInstance *testing.T) *http.Client {
        testingInstance.Helper()
        return &http.Client{
                Transport: roundTripperFunc(func(httpRequest *http.Request) (*http.Response, error) {
                       switch {
                       case httpRequest.URL.String() == proxy.ModelsURL():
                               body := `{"data":[{"id":"` + unsupportedModelIdentifier + `"}]}`
                               return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
                       case strings.HasPrefix(httpRequest.URL.String(), proxy.ModelsURL()+"/"):
                               return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(metadataEmpty)), Header: make(http.Header)}, nil
                       default:
                               testingInstance.Fatalf("unexpected request to %s", httpRequest.URL.String())
                               return nil, nil
                       }
                }),
                Timeout: 5 * time.Second,
        }
}

// TestIntegrationWebSearchUnsupportedModelReturnsBadRequest verifies that web search requests for unsupported models yield an HTTP 400 response.
func TestIntegrationWebSearchUnsupportedModelReturnsBadRequest(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	testCases := []struct{ name string }{{name: "web_search_unsupported"}}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			client := makeWebSearchRejectingHTTPClient(subTest)
			configureProxy(subTest, client)
			router, buildError := proxy.BuildRouter(proxy.Configuration{ServiceSecret: serviceSecretValue, OpenAIKey: openAIKeyValue, LogLevel: logLevelDebug, WorkerCount: 1, QueueSize: 8}, newLogger(subTest))
			if buildError != nil {
				subTest.Fatalf("BuildRouter failed: %v", buildError)
			}
			server := httptest.NewServer(router)
			subTest.Cleanup(server.Close)
			requestURL, _ := url.Parse(server.URL)
			queryValues := requestURL.Query()
			queryValues.Set(promptQueryParameter, promptValue)
			queryValues.Set(keyQueryParameter, serviceSecretValue)
			queryValues.Set(modelQueryParameter, unsupportedModelIdentifier)
			queryValues.Set(webSearchQueryParameter, "1")
			requestURL.RawQuery = queryValues.Encode()
			httpResponse, requestError := http.Get(requestURL.String())
			if requestError != nil {
				subTest.Fatalf("GET failed: %v", requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != http.StatusBadRequest {
				subTest.Fatalf("status=%d want=%d", httpResponse.StatusCode, http.StatusBadRequest)
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			responseText := strings.TrimSpace(string(responseBytes))
			if responseText != unsupportedErrorMessage {
				subTest.Fatalf("body=%q want=%q", responseText, unsupportedErrorMessage)
			}
		})
	}
}
