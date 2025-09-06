package proxy

import (
	"crypto/hmac"
	"crypto/sha256"
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
	expectedSecretFingerprint := utils.Fingerprint(normalizedSecret)
	return func(ginContext *gin.Context) {
		presentedKey := strings.TrimSpace(ginContext.Query(queryParameterKey))
		if !constantTimeEquals(normalizedSecret, presentedKey) {
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

// constantTimeEquals compares two string values using HMAC equality on SHA-256 hashes.
func constantTimeEquals(firstValue string, secondValue string) bool {
	firstDigest := sha256.Sum256([]byte(firstValue))
	secondDigest := sha256.Sum256([]byte(secondValue))
	return hmac.Equal(firstDigest[:], secondDigest[:])
}
