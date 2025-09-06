package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/constants"
	"go.uber.org/zap"
)

// result holds the outcome returned by a worker, including the text response
// and any error encountered during the OpenAI request.
type result struct {
	text         string
	requestError error
}

// requestTask carries all details needed to process a user request in the
// worker queue.
type requestTask struct {
	prompt           string
	systemPrompt     string
	model            string
	webSearchEnabled bool
	reply            chan result
}

// BuildRouter constructs the HTTP router used by the proxy. configuration supplies queue sizes, worker counts, timeout values, API credentials and other settings. structuredLogger records structured log messages during routing.
func BuildRouter(configuration Configuration, structuredLogger *zap.SugaredLogger) (*gin.Engine, error) {
	if validationError := validateConfig(configuration); validationError != nil {
		return nil, validationError
	}

	configuration.ApplyTunables()

	validator, validatorError := newModelValidator()
	if validatorError != nil {
		return nil, validatorError
	}

	if strings.ToLower(configuration.LogLevel) == LogLevelDebug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	if normalizedLogLevel := strings.ToLower(configuration.LogLevel); normalizedLogLevel == LogLevelInfo || normalizedLogLevel == LogLevelDebug {
		router.Use(requestResponseLogger(structuredLogger))
	}

	taskQueue := make(chan requestTask, configuration.QueueSize)
	openAIClient := NewOpenAIClient(HTTPClient, DefaultEndpoints, maxOutputTokens, UpstreamPollTimeout())
	for workerIndex := 0; workerIndex < configuration.WorkerCount; workerIndex++ {
		go func() {
			for pending := range taskQueue {
				text, requestError := openAIClient.openAIRequest(
					configuration.OpenAIKey,
					pending.model,
					pending.prompt,
					pending.systemPrompt,
					pending.webSearchEnabled,
					structuredLogger,
				)
				pending.reply <- result{text: text, requestError: requestError}
			}
		}()
	}

	router.Use(gin.Recovery(), secretMiddleware(configuration.ServiceSecret, structuredLogger))
	router.GET(rootPath, chatHandler(taskQueue, configuration.SystemPrompt, validator, structuredLogger))
	return router, nil
}

// Serve builds the router from the supplied configuration and structuredLogger and starts the HTTP server on the configured port.
func Serve(configuration Configuration, structuredLogger *zap.SugaredLogger) error {
	router, buildError := BuildRouter(configuration, structuredLogger)
	if buildError != nil {
		return buildError
	}
	return router.Run(fmt.Sprintf(":%d", configuration.Port))
}

// chatHandler returns a handler that forwards requests to the task queue.
func chatHandler(taskQueue chan requestTask, defaultSystemPrompt string, validator *modelValidator, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		userPrompt := ginContext.Query(queryParameterPrompt)
		if userPrompt == constants.EmptyString {
			ginContext.String(http.StatusBadRequest, errorMissingPrompt)
			return
		}

		systemPrompt := ginContext.Query(queryParameterSystemPrompt)
		if systemPrompt == constants.EmptyString {
			systemPrompt = defaultSystemPrompt
		}

		modelIdentifier := ginContext.Query(queryParameterModel)
		if modelIdentifier == constants.EmptyString {
			modelIdentifier = DefaultModel
		}
		if verificationError := validator.Verify(modelIdentifier); verificationError != nil {
			ginContext.String(http.StatusBadRequest, verificationError.Error())
			return
		}

		webSearchQuery := strings.TrimSpace(ginContext.Query(queryParameterWebSearch))
		webSearchEnabled := false
		if webSearchQuery != constants.EmptyString {
			parsedWebSearch, parseError := strconv.ParseBool(webSearchQuery)
			if parseError != nil {
				structuredLogger.Warnw(
					logEventParseWebSearchParameterFailed,
					logFieldValue, webSearchQuery,
					constants.LogFieldError, parseError,
				)
			} else {
				webSearchEnabled = parsedWebSearch
			}
		}

		replyChannel := make(chan result, 1)
		requestDeadline, deadlineFound := ginContext.Request.Context().Deadline()
		enqueueDuration := requestTimeout
		if deadlineFound {
			enqueueDuration = time.Until(requestDeadline)
		}
		enqueueContext, enqueueCancel := context.WithTimeout(ginContext.Request.Context(), enqueueDuration)
		select {
		case taskQueue <- requestTask{
			prompt:           userPrompt,
			systemPrompt:     systemPrompt,
			model:            modelIdentifier,
			webSearchEnabled: webSearchEnabled,
			reply:            replyChannel,
		}:
			enqueueCancel()
		case <-enqueueContext.Done():
			enqueueCancel()
			ginContext.String(http.StatusServiceUnavailable, errorQueueFull)
			return
		}

		requestContext, requestCancel := context.WithTimeout(ginContext.Request.Context(), requestTimeout)
		select {
		case outcome := <-replyChannel:
			requestCancel()
			if outcome.requestError != nil {
				if errors.Is(outcome.requestError, ErrUnknownModel) {
					ginContext.String(http.StatusBadRequest, outcome.requestError.Error())
				} else if errors.Is(outcome.requestError, context.DeadlineExceeded) {
					ginContext.String(http.StatusGatewayTimeout, errorRequestTimedOut)
				} else {
					ginContext.String(http.StatusBadGateway, outcome.requestError.Error())
				}
				return
			}
			mime := preferredMime(ginContext)
			formattedBody, contentType := formatResponse(outcome.text, mime, userPrompt, structuredLogger)
			ginContext.Data(http.StatusOK, contentType, []byte(formattedBody))
		case <-requestContext.Done():
			requestCancel()
			ginContext.String(http.StatusGatewayTimeout, errorRequestTimedOut)
		}
	}
}
