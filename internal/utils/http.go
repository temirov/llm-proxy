package utils

import (
	"io"
	"net/http"
	"time"

	"github.com/temirov/llm-proxy/internal/logging"
	"go.uber.org/zap"
)

// BuildHTTPRequestWithHeaders constructs an HTTP request and applies headers.
func BuildHTTPRequestWithHeaders(method string, requestURL string, body io.Reader, headers map[string]string) (*http.Request, error) {
	httpRequest, httpRequestError := http.NewRequest(method, requestURL, body)
	if httpRequestError != nil {
		return nil, httpRequestError
	}
	for headerName, headerValue := range headers {
		httpRequest.Header.Set(headerName, headerValue)
	}
	return httpRequest, nil
}

// PerformHTTPRequest executes the request with the provided do function and returns status, body, and latency.
func PerformHTTPRequest(do func(*http.Request) (*http.Response, error), httpRequest *http.Request, structuredLogger *zap.SugaredLogger, logEventOnTransportError string) (int, []byte, int64, error) {
	startTime := time.Now()
	httpResponse, httpError := do(httpRequest)
	latencyMilliseconds := time.Since(startTime).Milliseconds()
	if httpError != nil {
		if structuredLogger != nil {
			structuredLogger.Errorw(logEventOnTransportError, "err", httpError, logging.LogFieldLatencyMilliseconds, latencyMilliseconds)
		}
		return 0, nil, latencyMilliseconds, httpError
	}
	defer httpResponse.Body.Close()

	responseBytes, readError := io.ReadAll(httpResponse.Body)
	if readError != nil {
		if structuredLogger != nil {
			structuredLogger.Errorw(logging.LogEventReadResponseBodyFailed, "err", readError)
		}
		return httpResponse.StatusCode, nil, latencyMilliseconds, readError
	}
	return httpResponse.StatusCode, responseBytes, latencyMilliseconds, nil
}
