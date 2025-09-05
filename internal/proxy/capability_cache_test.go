package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

const (
	cacheRefreshModelIdentifier = "refresh-test-model"
	cacheRefreshModelList       = `{"data":[{"id":"%s"}]}`
	cacheRefreshCapabilities    = `{"allowed_request_fields":["temperature"]}`
	cacheRefreshErrorInit       = "validator initialization failed: %v"
	cacheRefreshErrorNoRefresh  = "cache was not refreshed"
	cacheRefreshErrorUnexpected = "unexpected refresh"
	cacheRefreshErrorMissing    = "temperature support missing"
)

// TestResolveModelSpecificationCacheRefresh verifies that expired cache entries trigger a refresh.
func TestResolveModelSpecificationCacheRefresh(testingInstance *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		requestCount++
		switch httpRequest.URL.Path {
		case "/":
			fmt.Fprintf(responseWriter, cacheRefreshModelList, cacheRefreshModelIdentifier)
		case "/" + cacheRefreshModelIdentifier:
			fmt.Fprint(responseWriter, cacheRefreshCapabilities)
		default:
			responseWriter.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	SetModelsURL(server.URL)
	defer ResetModelsURL()

	HTTPClient = server.Client()
	defer func() { HTTPClient = http.DefaultClient }()

	logger := zap.NewNop().Sugar()
	if _, initializationError := newModelValidator("key", logger); initializationError != nil {
		testingInstance.Fatalf(cacheRefreshErrorInit, initializationError)
	}

	modelCapabilityCache.cacheMutex.Lock()
	modelCapabilityCache.expiry = time.Now().Add(-time.Minute)
	modelCapabilityCache.cacheMutex.Unlock()

	initialRequests := requestCount
	capabilities := ResolveModelSpecification(cacheRefreshModelIdentifier)
	if !capabilities.SupportsTemperature() {
		testingInstance.Fatalf(cacheRefreshErrorMissing)
	}
	if requestCount <= initialRequests {
		testingInstance.Fatalf(cacheRefreshErrorNoRefresh)
	}

	requestsAfterRefresh := requestCount
	capabilities = ResolveModelSpecification(cacheRefreshModelIdentifier)
	if !capabilities.SupportsTemperature() {
		testingInstance.Fatalf(cacheRefreshErrorMissing)
	}
	if requestCount != requestsAfterRefresh {
		testingInstance.Fatalf(cacheRefreshErrorUnexpected)
	}
}
