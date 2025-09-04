package proxy

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/constants"
	"github.com/temirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

// sanitizeRequestURI replaces sensitive query parameter values with a placeholder.
func sanitizeRequestURI(requestURL *url.URL) string {
	queryParameters := requestURL.Query()
	if queryParameters.Has(queryParameterKey) {
		queryParameters.Set(queryParameterKey, redactedPlaceholder)
	}
	sanitizedURL := *requestURL
	sanitizedURL.RawQuery = queryParameters.Encode()
	return sanitizedURL.RequestURI()
}

// requestResponseLogger emits structured request and response metadata for traceability.
func requestResponseLogger(structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestStart := time.Now()
		requestMethod := ginContext.Request.Method
		requestPath := sanitizeRequestURI(ginContext.Request.URL)
		requestClientIP := ginContext.ClientIP()

		structuredLogger.Infow(
			logEventRequestReceived,
			logFieldMethod, requestMethod,
			logFieldPath, requestPath,
			logFieldClientIP, requestClientIP,
		)

		ginContext.Next()

		responseStatus := ginContext.Writer.Status()
		responseLatencyMillis := time.Since(requestStart).Milliseconds()
		structuredLogger.Infow(
			logEventResponseSent,
			logFieldStatus, responseStatus,
			constants.LogFieldLatencyMilliseconds, responseLatencyMillis,
		)
	}
}

// secretMiddleware enforces the shared secret through a constant-time comparison of the `key` query parameter.
func secretMiddleware(sharedSecret string, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	normalizedSecret := strings.TrimSpace(sharedSecret)
	return func(ginContext *gin.Context) {
		presentedKey := strings.TrimSpace(ginContext.Query(queryParameterKey))
		if !constantTimeEquals(normalizedSecret, presentedKey) {
			structuredLogger.Warnw(
				logEventForbiddenRequest,
				"expected_fingerprint", utils.Fingerprint(normalizedSecret),
			)
			ginContext.String(http.StatusForbidden, errorMissingClientKey)
			ginContext.Abort()
			return
		}
		ginContext.Next()
	}
}

// constantTimeEquals compares two strings in constant time to reduce side-channel signal.
func constantTimeEquals(first string, second string) bool {
	if len(first) != len(second) {
		_ = subtle.ConstantTimeCompare([]byte(first), []byte(first))
		_ = subtle.ConstantTimeCompare([]byte(second), []byte(first))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(first), []byte(second)) == 1
}
