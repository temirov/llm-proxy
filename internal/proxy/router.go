package proxy

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type result struct {
	text string
	err  error
}

type requestTask struct {
	prompt           string
	systemPrompt     string
	model            string
	webSearchEnabled bool
	reply            chan result
}

func BuildRouter(config Configuration, structuredLogger *zap.SugaredLogger) (*gin.Engine, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	// Normalize tunables with defaults
	if config.RequestTimeoutSeconds <= 0 {
		config.RequestTimeoutSeconds = DefaultRequestTimeoutSeconds
	}
	if config.UpstreamPollTimeoutSeconds <= 0 {
		config.UpstreamPollTimeoutSeconds = DefaultUpstreamPollTimeoutSeconds
	}
	if config.MaxOutputTokens <= 0 {
		config.MaxOutputTokens = DefaultMaxOutputTokens
	}

	// Apply tunables to package-level knobs
	requestTimeout = time.Duration(config.RequestTimeoutSeconds) * time.Second
	upstreamPollTimeout = time.Duration(config.UpstreamPollTimeoutSeconds) * time.Second
	maxOutputTokens = config.MaxOutputTokens

	validator, validatorError := newModelValidator(config.OpenAIKey, structuredLogger)
	if validatorError != nil {
		return nil, validatorError
	}

	if strings.ToLower(config.LogLevel) == LogLevelDebug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	if lvl := strings.ToLower(config.LogLevel); lvl == LogLevelInfo || lvl == LogLevelDebug {
		router.Use(requestResponseLogger(structuredLogger))
	}

	taskQueue := make(chan requestTask, config.QueueSize)
	for workerIndex := 0; workerIndex < config.WorkerCount; workerIndex++ {
		go func() {
			for pending := range taskQueue {
				text, requestError := openAIRequest(
					config.OpenAIKey,
					pending.model,
					pending.prompt,
					pending.systemPrompt,
					pending.webSearchEnabled,
					structuredLogger,
				)
				pending.reply <- result{text: text, err: requestError}
			}
		}()
	}

	router.Use(gin.Recovery(), secretMiddleware(config.ServiceSecret, structuredLogger))
	router.GET("/", chatHandler(taskQueue, config.SystemPrompt, validator, structuredLogger))
	return router, nil
}

func Serve(config Configuration, structuredLogger *zap.SugaredLogger) error {
	router, buildError := BuildRouter(config, structuredLogger)
	if buildError != nil {
		return buildError
	}
	return router.Run(fmt.Sprintf(":%d", config.Port))
}

func chatHandler(taskQueue chan requestTask, defaultSystemPrompt string, validator *modelValidator, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		userPrompt := ginContext.Query(queryParameterPrompt)
		if userPrompt == "" {
			ginContext.String(http.StatusBadRequest, errorMissingPrompt)
			return
		}

		systemPrompt := ginContext.Query(queryParameterSystemPrompt)
		if systemPrompt == "" {
			systemPrompt = defaultSystemPrompt
		}

		modelIdentifier := ginContext.Query(queryParameterModel)
		if modelIdentifier == "" {
			modelIdentifier = DefaultModel
		}
		if err := validator.Verify(modelIdentifier); err != nil {
			ginContext.String(http.StatusBadRequest, err.Error())
			return
		}

		webSearchQuery := strings.TrimSpace(ginContext.Query(queryParameterWebSearch))
		webSearchEnabled := false
		if webSearchQuery != "" {
			parsedWebSearch, parseError := strconv.ParseBool(webSearchQuery)
			if parseError != nil {
				structuredLogger.Warnw(
					logEventParseWebSearchParameterFailed,
					logFieldValue, webSearchQuery,
					logFieldError, parseError,
				)
			} else {
				webSearchEnabled = parsedWebSearch
			}
		}
		if webSearchEnabled && mustRejectWebSearchAtIngress(modelIdentifier) {
			ginContext.String(http.StatusBadRequest, errorWebSearchUnsupportedByModel)
			return
		}

		replyChannel := make(chan result, 1)
		taskQueue <- requestTask{
			prompt:           userPrompt,
			systemPrompt:     systemPrompt,
			model:            modelIdentifier,
			webSearchEnabled: webSearchEnabled,
			reply:            replyChannel,
		}

		select {
		case outcome := <-replyChannel:
			if outcome.err != nil {
				if strings.Contains(outcome.err.Error(), "unknown model") {
					ginContext.String(http.StatusBadRequest, outcome.err.Error())
				} else {
					ginContext.String(http.StatusBadGateway, outcome.err.Error())
				}
				return
			}
			mime := preferredMime(ginContext)
			formattedBody, contentType := formatResponse(outcome.text, mime, userPrompt, structuredLogger)
			ginContext.Data(http.StatusOK, contentType, []byte(formattedBody))
		case <-time.After(requestTimeout):
			ginContext.String(http.StatusGatewayTimeout, errorRequestTimedOut)
		}
	}
}
