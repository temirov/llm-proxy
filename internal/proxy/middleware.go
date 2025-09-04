package proxy

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

func sanitizeRequestURI(u *url.URL) string {
	q := u.Query()
	if q.Has(queryParameterKey) {
		q.Set(queryParameterKey, "***REDACTED***")
	}
	u2 := *u
	u2.RawQuery = q.Encode()
	return u2.RequestURI()
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
			logFieldLatencyMs, responseLatencyMillis,
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
			ginContext.AbortWithStatus(http.StatusForbidden)
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
