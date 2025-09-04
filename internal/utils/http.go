package utils

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/temirov/llm-proxy/internal/constants"
	"go.uber.org/zap"
)

var exponentialBackoffPool = sync.Pool{
	New: func() any {
		return backoff.NewExponentialBackOff()
	},
}

// AcquireExponentialBackoff retrieves a reusable exponential backoff instance.
func AcquireExponentialBackoff() *backoff.ExponentialBackOff {
	return exponentialBackoffPool.Get().(*backoff.ExponentialBackOff)
}

// ReleaseExponentialBackoff resets the backoff and returns it to the pool.
func ReleaseExponentialBackoff(exponentialBackoff *backoff.ExponentialBackOff) {
	exponentialBackoff.Reset()
	exponentialBackoffPool.Put(exponentialBackoff)
}

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

// PerformHTTPRequest issues the HTTP request and returns the status code, body, and latency.
// It automatically retries transport failures using exponential backoff.
func PerformHTTPRequest(do func(*http.Request) (*http.Response, error), httpRequest *http.Request, structuredLogger *zap.SugaredLogger, logEventOnTransportError string) (int, []byte, int64, error) {
	startTime := time.Now()
	var httpResponse *http.Response
	operation := func() error {
		if httpRequest.GetBody != nil {
			resetBody, resetError := httpRequest.GetBody()
			if resetError != nil {
				return resetError
			}
			httpRequest.Body = resetBody
		}
		response, httpError := do(httpRequest)
		if httpError != nil {
			if structuredLogger != nil {
				structuredLogger.Errorw(logEventOnTransportError, constants.LogFieldError, httpError)
			}
			return httpError
		}
		httpResponse = response
		return nil
	}

	exponentialBackoff := AcquireExponentialBackoff()
	defer ReleaseExponentialBackoff(exponentialBackoff)
	retryError := backoff.Retry(operation, backoff.WithContext(exponentialBackoff, httpRequest.Context()))
	latencyMillis := time.Since(startTime).Milliseconds()
	if retryError != nil {
		if structuredLogger != nil {
			structuredLogger.Errorw(
				logEventOnTransportError,
				constants.LogFieldError,
				retryError,
				constants.LogFieldLatencyMilliseconds,
				latencyMillis,
			)
		}
		return 0, nil, latencyMillis, retryError
	}
	defer httpResponse.Body.Close()

	responseBytes, readError := io.ReadAll(httpResponse.Body)
	if readError != nil {
		if structuredLogger != nil {
			structuredLogger.Errorw(constants.LogEventReadResponseBodyFailed, constants.LogFieldError, readError)
		}
		return httpResponse.StatusCode, nil, latencyMillis, readError
	}
	return httpResponse.StatusCode, responseBytes, latencyMillis, nil
}
