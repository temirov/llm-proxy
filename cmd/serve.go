package cmd

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	openAIResponsesURL = "https://api.openai.com/v1/responses"
	openAIModelsURL    = "https://api.openai.com/v1/models"
	defaultPort        = 8080
	defaultWorkers     = 4
	defaultQueueSize   = 100
	defaultModel       = "gpt-4.1"
	modelsCacheTTL     = 24 * time.Hour
)

var requestTimeout = 30 * time.Second

type modelValidator struct {
	mu     sync.RWMutex
	models map[string]struct{}
	expiry time.Time
	apiKey string
	logger *zap.SugaredLogger
}

// Configuration aggregates runtime settings.
type Configuration struct {
	ServiceSecret string
	OpenAIKey     string
	Port          int
	LogLevel      string // "debug", "info", or "none"
	SystemPrompt  string
	WorkerCount   int
	QueueSize     int
}

type responsesAPIShim struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

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

func newModelValidator(openAIKey string, logger *zap.SugaredLogger) (*modelValidator, error) {
	validator := &modelValidator{apiKey: openAIKey, logger: logger}
	if err := validator.refresh(); err != nil {
		return nil, err
	}
	return validator, nil
}

func (validator *modelValidator) refresh() error {
	request, _ := http.NewRequest(http.MethodGet, openAIModelsURL, nil)
	request.Header.Set("Authorization", "Bearer "+validator.apiKey)

	startTime := time.Now()
	response, requestErr := http.DefaultClient.Do(request)
	latency := time.Since(startTime).Milliseconds()
	if requestErr != nil {
		validator.logger.Errorw("OpenAI models list error", "err", requestErr, "latency_ms", latency)
		return requestErr
	}
	defer response.Body.Close()
	validator.logger.Infow("OpenAI models list", "status", response.StatusCode, "latency_ms", latency)
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("OpenAI models list error")
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return err
	}
	modelSet := make(map[string]struct{}, len(payload.Data))
	for _, modelInfo := range payload.Data {
		modelSet[modelInfo.ID] = struct{}{}
	}
	validator.mu.Lock()
	validator.models = modelSet
	validator.expiry = time.Now().Add(modelsCacheTTL)
	validator.mu.Unlock()
	return nil
}

func (validator *modelValidator) Verify(model string) error {
	validator.mu.RLock()
	cacheExpiry := validator.expiry
	_, present := validator.models[model]
	validator.mu.RUnlock()
	if time.Now().After(cacheExpiry) || validator.models == nil {
		if err := validator.refresh(); err != nil {
			return fmt.Errorf("OpenAI model validation error")
		}
		validator.mu.RLock()
		_, present = validator.models[model]
		validator.mu.RUnlock()
	}
	if !present {
		return fmt.Errorf("unknown model: %s", model)
	}
	return nil
}

// requestResponseLogger logs request arrival and response metadata at INFO.
func requestResponseLogger(logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(context *gin.Context) {
		startTime := time.Now()
		requestMethod := context.Request.Method
		requestPath := context.Request.URL.RequestURI()
		clientAddress := context.ClientIP()

		logger.Infow("request received",
			"method", requestMethod,
			"path", requestPath,
			"client_ip", clientAddress,
		)

		context.Next()

		httpStatus := context.Writer.Status()
		latency := time.Since(startTime).Milliseconds()
		logger.Infow("response sent",
			"status", httpStatus,
			"latency_ms", latency,
		)
	}
}

// serve sets up Gin with conditional logging and starts the server.
func serve(config Configuration, logger *zap.SugaredLogger) error {
	if err := validateConfig(config); err != nil {
		return err
	}

	validator, validatorErr := newModelValidator(config.OpenAIKey, logger)
	if validatorErr != nil {
		return validatorErr
	}

	if config.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	if config.LogLevel == "info" || config.LogLevel == "debug" {
		router.Use(requestResponseLogger(logger))
	}

	taskQueue := make(chan requestTask, config.QueueSize)
	for workerIndex := 0; workerIndex < config.WorkerCount; workerIndex++ {
		go func() {
			for pendingTask := range taskQueue {
				responseText, requestErr := openAIRequest(
					config.OpenAIKey,
					pendingTask.model,
					pendingTask.prompt,
					pendingTask.systemPrompt,
					pendingTask.webSearchEnabled,
					logger,
				)
				pendingTask.reply <- result{text: responseText, err: requestErr}
			}
		}()
	}

	router.Use(gin.Recovery(), secretMiddleware(config.ServiceSecret, logger))
	router.GET("/", chatHandler(taskQueue, config.SystemPrompt, validator, logger))
	return router.Run(fmt.Sprintf(":%d", config.Port))
}

// validateConfig ensures all required Configuration fields are present.
func validateConfig(config Configuration) error {
	if config.ServiceSecret == "" {
		return fmt.Errorf("SERVICE_SECRET must be set")
	}
	if config.OpenAIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY must be set")
	}
	return nil
}

// secretMiddleware rejects requests that do not provide the correct
// `key` query parameter.
func secretMiddleware(secret string, logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(context *gin.Context) {
		if context.Query("key") != secret {
			logger.Warnw("forbidden request", "presented_key", context.Query("key"))
			context.AbortWithStatus(http.StatusForbidden)
			return
		}
		context.Next()
	}
}

func openAIRequest(openAIKey, model, prompt, systemPrompt string, webSearchEnabled bool, logger *zap.SugaredLogger) (string, error) {
	messageArray := []map[string]string{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": prompt},
	}

	requestPayload := map[string]any{
		"model":             model,
		"input":             messageArray,
		"temperature":       0.7,
		"max_output_tokens": 1024,
	}

	if webSearchEnabled {
		requestPayload["tools"] = []any{
			map[string]any{"type": "web_search"},
		}
	}

	bodyBytes, _ := json.Marshal(requestPayload)
	request, _ := http.NewRequest(http.MethodPost, openAIResponsesURL, bytes.NewReader(bodyBytes))
	request.Header.Set("Authorization", "Bearer "+openAIKey)
	request.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	response, err := http.DefaultClient.Do(request)
	latency := time.Since(startTime).Milliseconds()
	if err != nil {
		logger.Errorw("OpenAI request error", "err", err, "latency_ms", latency)
		return "", fmt.Errorf("OpenAI request error")
	}
	defer response.Body.Close()
	responseBytes, _ := io.ReadAll(response.Body)

	responseText := ""
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		var responsesShape map[string]any
		if json.Unmarshal(responseBytes, &responsesShape) == nil {
			if direct, ok := responsesShape["output_text"].(string); ok && direct != "" {
				responseText = direct
			} else {
				var shim responsesAPIShim
				if json.Unmarshal(responseBytes, &shim) == nil && len(shim.Choices) > 0 {
					responseText = shim.Choices[0].Message.Content
				}
			}
		}
	}

	logger.Infow("OpenAI API response",
		"status", response.StatusCode,
		"latency_ms", latency,
		"response_text", responseText,
	)

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		logger.Errorw("OpenAI API error", "status", response.StatusCode, "body", string(responseBytes))
		return "", fmt.Errorf("OpenAI API error")
	}

	return responseText, nil
}

// preferredMime returns the client's requested MIME type via the "format"
// query parameter or the Accept header.
func preferredMime(ctx *gin.Context) string {
	if formatParam := ctx.Query("format"); formatParam != "" {
		return strings.ToLower(strings.TrimSpace(formatParam))
	}
	return strings.ToLower(strings.TrimSpace(ctx.GetHeader("Accept")))
}

// formatResponse converts the model output to the requested MIME type and
// returns the formatted body along with its Content-Type.
func formatResponse(text, mime, prompt string) (string, string) {
	switch {
	case strings.Contains(mime, "application/json"):
		encoded, _ := json.Marshal(map[string]string{"request": prompt, "response": text})
		return string(encoded), "application/json"
	case strings.Contains(mime, "application/xml"), strings.Contains(mime, "text/xml"):
		type xmlResponse struct {
			XMLName xml.Name `xml:"response"`
			Request string   `xml:"request,attr"`
			Text    string   `xml:",chardata"`
		}
		xmlResp := xmlResponse{Request: prompt, Text: text}
		encoded, _ := xml.Marshal(xmlResp)
		return string(encoded), "application/xml"
	case strings.Contains(mime, "text/csv"):
		escaped := strings.ReplaceAll(text, "\"", "\"\"")
		return fmt.Sprintf("\"%s\"\n", escaped), "text/csv"
	default:
		return text, "text/plain; charset=utf-8"
	}
}

// chatHandler processes chat requests by dispatching them to the worker queue
// and returning the formatted response or an error to the client.
func chatHandler(taskQueue chan requestTask, systemPrompt string, validator *modelValidator, logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(context *gin.Context) {
		userPrompt := context.Query("prompt")
		if userPrompt == "" {
			context.String(http.StatusBadRequest, "missing prompt parameter")
			return
		}

		systemPromptOverride := context.Query("system_prompt")
		if systemPromptOverride == "" {
			systemPromptOverride = systemPrompt
		}

		modelParam := context.Query("model")
		if modelParam == "" {
			modelParam = defaultModel
		}
		if err := validator.Verify(modelParam); err != nil {
			context.String(http.StatusBadRequest, err.Error())
			return
		}

		webSearchParam := strings.TrimSpace(strings.ToLower(context.Query("web_search")))
		webSearchEnabled := webSearchParam == "1" || webSearchParam == "true" || webSearchParam == "yes"

		replyChannel := make(chan result, 1)
		taskQueue <- requestTask{
			prompt:           userPrompt,
			systemPrompt:     systemPromptOverride,
			model:            modelParam,
			webSearchEnabled: webSearchEnabled,
			reply:            replyChannel,
		}

		select {
		case computation := <-replyChannel:
			if computation.err != nil {
				if strings.Contains(computation.err.Error(), "unknown model") {
					context.String(http.StatusBadRequest, computation.err.Error())
				} else {
					context.String(http.StatusBadGateway, computation.err.Error())
				}
			} else {
				requestedMime := preferredMime(context)
				formattedBody, contentType := formatResponse(computation.text, requestedMime, userPrompt)
				context.Data(http.StatusOK, contentType, []byte(formattedBody))
			}
		case <-time.After(requestTimeout):
			context.String(http.StatusGatewayTimeout, "request timed out")
		}
	}
}
