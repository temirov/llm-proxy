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
	openAIURL        = "https://api.openai.com/v1/chat/completions"
	openAIModelsURL  = "https://api.openai.com/v1/models"
	defaultPort      = 8080
	defaultWorkers   = 4
	defaultQueueSize = 100
	defaultModel     = "gpt-4.1"
	modelsCacheTTL   = 24 * time.Hour
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
	prompt       string
	systemPrompt string
	model        string
	reply        chan result
}

func newModelValidator(openAIKey string, logger *zap.SugaredLogger) (*modelValidator, error) {
	v := &modelValidator{apiKey: openAIKey, logger: logger}
	if err := v.refresh(); err != nil {
		return nil, err
	}
	return v, nil
}

func (v *modelValidator) refresh() error {
	request, _ := http.NewRequest(http.MethodGet, openAIModelsURL, nil)
	request.Header.Set("Authorization", "Bearer "+v.apiKey)

	start := time.Now()
	response, err := http.DefaultClient.Do(request)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		v.logger.Errorw("OpenAI models list error", "err", err, "latency_ms", latency)
		return err
	}
	defer response.Body.Close()
	v.logger.Infow("OpenAI models list", "status", response.StatusCode, "latency_ms", latency)
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
	models := make(map[string]struct{}, len(payload.Data))
	for _, m := range payload.Data {
		models[m.ID] = struct{}{}
	}
	v.mu.Lock()
	v.models = models
	v.expiry = time.Now().Add(modelsCacheTTL)
	v.mu.Unlock()
	return nil
}

func (v *modelValidator) Verify(model string) error {
	v.mu.RLock()
	expiry := v.expiry
	_, ok := v.models[model]
	v.mu.RUnlock()
	if time.Now().After(expiry) || v.models == nil {
		if err := v.refresh(); err != nil {
			return fmt.Errorf("OpenAI model validation error")
		}
		v.mu.RLock()
		_, ok = v.models[model]
		v.mu.RUnlock()
	}
	if !ok {
		return fmt.Errorf("unknown model: %s", model)
	}
	return nil
}

// requestResponseLogger logs request arrival and response metadata at INFO.
func requestResponseLogger(logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(context *gin.Context) {
		start := time.Now()
		method := context.Request.Method
		path := context.Request.URL.RequestURI()
		ip := context.ClientIP()

		logger.Infow("request received",
			"method", method,
			"path", path,
			"client_ip", ip,
		)

		context.Next()

		status := context.Writer.Status()
		latency := time.Since(start).Milliseconds()
		logger.Infow("response sent",
			"status", status,
			"latency_ms", latency,
		)
	}
}

// serve sets up Gin with conditional logging and starts the server.
func serve(config Configuration, logger *zap.SugaredLogger) error {
	if err := validateConfig(config); err != nil {
		return err
	}

	validator, err := newModelValidator(config.OpenAIKey, logger)
	if err != nil {
		return err
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
			for task := range taskQueue {
				text, err := openAIRequest(config.OpenAIKey, task.model, task.prompt, task.systemPrompt, logger)
				task.reply <- result{text: text, err: err}
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

// openAIRequest sends the prompt and system prompt to the OpenAI chat API
// and returns the resulting text.
func openAIRequest(openAIKey, model, prompt, systemPrompt string, logger *zap.SugaredLogger) (string, error) {
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
		"max_tokens":  1024,
	}
	bodyBytes, _ := json.Marshal(payload)
	request, _ := http.NewRequest(http.MethodPost, openAIURL, bytes.NewReader(bodyBytes))
	request.Header.Set("Authorization", "Bearer "+openAIKey)
	request.Header.Set("Content-Type", "application/json")

	start := time.Now()
	response, err := http.DefaultClient.Do(request)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		logger.Errorw("OpenAI request error", "err", err, "latency_ms", latency)
		return "", fmt.Errorf("OpenAI request error")
	}
	defer response.Body.Close()
	responseBytes, _ := io.ReadAll(response.Body)
	content := ""
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		var proxyResp proxyResponse
		if jsonErr := json.Unmarshal(responseBytes, &proxyResp); jsonErr == nil && len(proxyResp.Choices) > 0 {
			content = proxyResp.Choices[0].Message.Content
		}
	}

	logger.Infow("OpenAI API response",
		"status", response.StatusCode,
		"latency_ms", latency,
		"response_text", content,
	)

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		logger.Errorw("OpenAI API error", "status", response.StatusCode, "body", string(responseBytes))
		return "", fmt.Errorf("OpenAI API error")
	}

	return content, nil
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
		prompt := context.Query("prompt")
		if prompt == "" {
			context.String(http.StatusBadRequest, "missing prompt parameter")
			return
		}

		systemPromptOverride := context.Query("system_prompt")
		if systemPromptOverride == "" {
			systemPromptOverride = systemPrompt
		}

		model := context.Query("model")
		if model == "" {
			model = defaultModel
		}
		if err := validator.Verify(model); err != nil {
			context.String(http.StatusBadRequest, err.Error())
			return
		}

		reply := make(chan result, 1)
		taskQueue <- requestTask{prompt: prompt, systemPrompt: systemPromptOverride, model: model, reply: reply}

		select {
		case res := <-reply:
			if res.err != nil {
				if strings.Contains(res.err.Error(), "unknown model") {
					context.String(http.StatusBadRequest, res.err.Error())
				} else {
					context.String(http.StatusBadGateway, res.err.Error())
				}
			} else {
				mime := preferredMime(context)
				formatted, contentType := formatResponse(res.text, mime, prompt)
				context.Data(http.StatusOK, contentType, []byte(formatted))
			}
		case <-time.After(requestTimeout):
			context.String(http.StatusGatewayTimeout, "request timed out")
		}
	}
}
