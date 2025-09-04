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
	expectedSecretBytes := []byte(normalizedSecret)
	expectedSecretFingerprint := utils.Fingerprint(normalizedSecret)
	return func(ginContext *gin.Context) {
		presentedKey := strings.TrimSpace(ginContext.Query(queryParameterKey))
		presentedKeyBytes := []byte(presentedKey)
		if !constantTimeEquals(expectedSecretBytes, presentedKeyBytes) {
			structuredLogger.Warnw(
				logEventForbiddenRequest,
				logFieldExpectedFingerprint, expectedSecretFingerprint,
			)
			ginContext.String(http.StatusForbidden, errorMissingClientKey)
			ginContext.Abort()
			return
		}
		ginContext.Next()
	}
}

// constantTimeEquals compares two byte slices in constant time to reduce side-channel signal.
func constantTimeEquals(firstValue []byte, secondValue []byte) bool {
	if len(firstValue) != len(secondValue) {
		_ = subtle.ConstantTimeCompare(firstValue, firstValue)
		_ = subtle.ConstantTimeCompare(secondValue, firstValue)
		return false
	}
	return subtle.ConstantTimeCompare(firstValue, secondValue) == 1
}
