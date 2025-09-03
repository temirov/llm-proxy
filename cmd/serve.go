package cmd

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	openAIURL        = "https://api.openai.com/v1/responses"
	openAIModelsURL  = "https://api.openai.com/v1/models"
	defaultPort      = 8080
	defaultWorkers   = 4
	defaultQueueSize = 100
)

var requestTimeout = 30 * time.Second

type Configuration struct {
	ServiceSecret string
	OpenAIKey     string
	Port          int
	LogLevel      string
	SystemPrompt  string
	WorkerCount   int
	QueueSize     int
}

type proxyResponse struct {
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
	webSearchEnabled bool
	reply            chan result
}

func requestResponseLogger(logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(context *gin.Context) {
		startTime := time.Now()
		requestMethod := context.Request.Method
		requestPath := context.Request.URL.RequestURI()
		clientAddress := context.ClientIP()

		logger.Infow(
			"request received",
			"method", requestMethod,
			"path", requestPath,
			"client_ip", clientAddress,
		)

		context.Next()

		httpStatus := context.Writer.Status()
		totalLatency := time.Since(startTime).Milliseconds()
		logger.Infow(
			"response sent",
			"status", httpStatus,
			"latency_ms", totalLatency,
		)
	}
}

func serve(config Configuration, logger *zap.SugaredLogger) error {
	if err := validateConfig(config); err != nil {
		return err
	}

	if config.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	httpRouter := gin.New()
	if config.LogLevel == "info" || config.LogLevel == "debug" {
		httpRouter.Use(requestResponseLogger(logger))
	}

	taskQueue := make(chan requestTask, config.QueueSize)
	for workerIndex := 0; workerIndex < config.WorkerCount; workerIndex++ {
		go func() {
			for pendingTask := range taskQueue {
				responseText, requestErr := openAIRequest(
					config.OpenAIKey,
					pendingTask.prompt,
					pendingTask.systemPrompt,
					pendingTask.webSearchEnabled,
					logger,
				)
				pendingTask.reply <- result{text: responseText, err: requestErr}
			}
		}()
	}

	httpRouter.Use(gin.Recovery(), secretMiddleware(config.ServiceSecret, logger))
	httpRouter.GET("/", chatHandler(taskQueue, config.SystemPrompt, logger))
	return httpRouter.Run(fmt.Sprintf(":%d", config.Port))
}

func validateConfig(config Configuration) error {
	if config.ServiceSecret == "" {
		return fmt.Errorf("SERVICE_SECRET must be set")
	}
	if config.OpenAIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY must be set")
	}
	apiRequest, requestErr := http.NewRequest(http.MethodGet, openAIModelsURL, nil)
	if requestErr != nil {
		return requestErr
	}
	apiRequest.Header.Set("Authorization", "Bearer "+config.OpenAIKey)
	httpClient := &http.Client{Timeout: 10 * time.Second}

	startTime := time.Now()
	apiResponse, doErr := httpClient.Do(apiRequest)
	totalLatency := time.Since(startTime).Milliseconds()
	if doErr == nil {
		defer apiResponse.Body.Close()
	}
	validationLogger := zap.NewExample().Sugar()
	httpStatus := 0
	if apiResponse != nil {
		httpStatus = apiResponse.StatusCode
	}
	validationLogger.Infow(
		"OpenAI key validation",
		"status", httpStatus,
		"latency_ms", totalLatency,
	)
	if doErr != nil {
		return fmt.Errorf("failed to validate OPENAI_API_KEY: %w", doErr)
	}
	if apiResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("OPENAI_API_KEY validation failed (status %d)", apiResponse.StatusCode)
	}
	return nil
}

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

func openAIRequest(openAIKey, prompt, systemPrompt string, webSearchEnabled bool, logger *zap.SugaredLogger) (string, error) {
	messages := []map[string]string{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": prompt},
	}

	payload := map[string]any{
		"model":             "gpt-4o",
		"input":             messages,
		"temperature":       0.7,
		"max_output_tokens": 1024,
	}

	if webSearchEnabled {
		payload["tools"] = []any{
			map[string]any{"type": "web_search"},
		}
	}

	requestBodyBytes, _ := json.Marshal(payload)
	httpRequest, _ := http.NewRequest(http.MethodPost, openAIURL, bytes.NewReader(requestBodyBytes))
	httpRequest.Header.Set("Authorization", "Bearer "+openAIKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	httpResponse, doErr := http.DefaultClient.Do(httpRequest)
	totalLatency := time.Since(startTime).Milliseconds()
	if doErr != nil {
		logger.Errorw("OpenAI request error", "err", doErr, "latency_ms", totalLatency)
		return "", fmt.Errorf("OpenAI request error")
	}
	defer httpResponse.Body.Close()
	responseBodyBytes, _ := io.ReadAll(httpResponse.Body)

	responseText := ""
	if httpResponse.StatusCode >= 200 && httpResponse.StatusCode < 300 {
		var generic map[string]any
		if json.Unmarshal(responseBodyBytes, &generic) == nil {
			if direct, ok := generic["output_text"].(string); ok && direct != "" {
				responseText = direct
			} else if outputArray, ok := generic["output"].([]any); ok {
				for _, outputItem := range outputArray {
					outputMap, isMap := outputItem.(map[string]any)
					if !isMap {
						continue
					}
					if outputMap["type"] == "message" {
						if contentArray, ok := outputMap["content"].([]any); ok {
							for _, contentItem := range contentArray {
								contentMap, ok := contentItem.(map[string]any)
								if ok && contentMap["type"] == "output_text" {
									if textValue, ok := contentMap["text"].(string); ok {
										responseText = textValue
										break
									}
								}
							}
						}
					}
				}
			} else {
				var legacy proxyResponse
				if json.Unmarshal(responseBodyBytes, &legacy) == nil && len(legacy.Choices) > 0 {
					responseText = legacy.Choices[0].Message.Content
				}
			}
		}
	}

	logger.Infow(
		"OpenAI API response",
		"status", httpResponse.StatusCode,
		"latency_ms", totalLatency,
		"response_text", responseText,
	)

	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		logger.Errorw("OpenAI API error", "status", httpResponse.StatusCode, "body", string(responseBodyBytes))
		return "", fmt.Errorf("OpenAI API error")
	}

	return responseText, nil
}

func preferredMime(ctx *gin.Context) string {
	if formatParam := ctx.Query("format"); formatParam != "" {
		return strings.ToLower(strings.TrimSpace(formatParam))
	}
	return strings.ToLower(strings.TrimSpace(ctx.GetHeader("Accept")))
}

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

func chatHandler(taskQueue chan requestTask, systemPrompt string, logger *zap.SugaredLogger) gin.HandlerFunc {
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

		webSearchParam := strings.TrimSpace(strings.ToLower(context.Query("web_search")))
		webSearchEnabled := webSearchParam == "1" || webSearchParam == "true" || webSearchParam == "yes"

		replyChannel := make(chan result, 1)
		taskQueue <- requestTask{
			prompt:           userPrompt,
			systemPrompt:     systemPromptOverride,
			webSearchEnabled: webSearchEnabled,
			reply:            replyChannel,
		}

		select {
		case computation := <-replyChannel:
			if computation.err != nil {
				context.String(http.StatusBadGateway, computation.err.Error())
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
