package utils

import (
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const (
	// logEventReadResponseBodyFailed identifies failures while reading an HTTP response body.
	logEventReadResponseBodyFailed = "read response body failed"

	// logFieldLatencyMs represents the log field key for HTTP latency in milliseconds.
	logFieldLatencyMs = "latency_ms"
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
	latencyMillis := time.Since(startTime).Milliseconds()
	if httpError != nil {
		if structuredLogger != nil {
			structuredLogger.Errorw(logEventOnTransportError, "err", httpError, logFieldLatencyMs, latencyMillis)
		}
		return 0, nil, latencyMillis, httpError
	}
	defer httpResponse.Body.Close()

	responseBytes, readError := io.ReadAll(httpResponse.Body)
	if readError != nil {
		if structuredLogger != nil {
			structuredLogger.Errorw(logEventReadResponseBodyFailed, "err", readError)
		}
		return httpResponse.StatusCode, nil, latencyMillis, readError
	}
	return httpResponse.StatusCode, responseBytes, latencyMillis, nil
}
