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
	openAIURL        = "https://api.openai.com/v1/chat/completions"
	openAIModelsURL  = "https://api.openai.com/v1/models"
	defaultPort      = 8080
	defaultWorkers   = 4
	defaultQueueSize = 100
)

var requestTimeout = 30 * time.Second

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
	reply        chan result
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
	for i := 0; i < config.WorkerCount; i++ {
		go func() {
			for task := range taskQueue {
				text, err := openAIRequest(config.OpenAIKey, task.prompt, task.systemPrompt, logger)
				task.reply <- result{text: text, err: err}
			}
		}()
	}

	router.Use(gin.Recovery(), secretMiddleware(config.ServiceSecret, logger))
	router.GET("/", chatHandler(taskQueue, config.SystemPrompt, logger))
	return router.Run(fmt.Sprintf(":%d", config.Port))
}

func validateConfig(config Configuration) error {
	if config.ServiceSecret == "" {
		return fmt.Errorf("SERVICE_SECRET must be set")
	}
	if config.OpenAIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY must be set")
	}
	request, err := http.NewRequest(http.MethodGet, openAIModelsURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+config.OpenAIKey)
	client := &http.Client{Timeout: 10 * time.Second}

	start := time.Now()
	response, err := client.Do(request)
	latency := time.Since(start).Milliseconds()
	if err == nil {
		defer response.Body.Close()
	}
	logger := zap.NewExample().Sugar()
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	logger.Infow("OpenAI key validation",
		"status", status,
		"latency_ms", latency,
	)
	if err != nil {
		return fmt.Errorf("failed to validate OPENAI_API_KEY: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("OPENAI_API_KEY validation failed (status %d)", response.StatusCode)
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

func openAIRequest(openAIKey, prompt, systemPrompt string, logger *zap.SugaredLogger) (string, error) {
	payload := map[string]any{
		"model": "gpt-4.1",
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

func preferredMime(c *gin.Context) string {
	if q := c.Query("format"); q != "" {
		return strings.ToLower(strings.TrimSpace(q))
	}
	return strings.ToLower(strings.TrimSpace(c.GetHeader("Accept")))
}

func formatResponse(text, mime, prompt string) (string, string) {
	switch {
	case strings.Contains(mime, "application/json"):
		b, _ := json.Marshal(map[string]string{"request": prompt, "response": text})
		return string(b), "application/json"
	case strings.Contains(mime, "application/xml"), strings.Contains(mime, "text/xml"):
		type xmlResp struct {
			XMLName xml.Name `xml:"response"`
			Request string   `xml:"request,attr"`
			Text    string   `xml:",chardata"`
		}
		r := xmlResp{Request: prompt, Text: text}
		b, _ := xml.Marshal(r)
		return string(b), "application/xml"
	case strings.Contains(mime, "text/csv"):
		escaped := strings.ReplaceAll(text, "\"", "\"\"")
		return fmt.Sprintf("\"%s\"\n", escaped), "text/csv"
	default:
		return text, "text/plain; charset=utf-8"
	}
}

func chatHandler(taskQueue chan requestTask, systemPrompt string, logger *zap.SugaredLogger) gin.HandlerFunc {
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

		reply := make(chan result, 1)
		taskQueue <- requestTask{prompt: prompt, systemPrompt: systemPromptOverride, reply: reply}

		select {
		case res := <-reply:
			if res.err != nil {
				context.String(http.StatusBadGateway, res.err.Error())
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
