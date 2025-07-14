package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	openAIURL       = "https://api.openai.com/v1/chat/completions"
	openAIModelsURL = "https://api.openai.com/v1/models"
	defaultPort     = 8080
)

// Configuration aggregates runtime settings.
type Configuration struct {
	ServiceSecret string
	OpenAIKey     string
	Port          int
	LogLevel      string // "debug", "info", or "none"
}

type proxyResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// requestResponseLogger logs request arrival and response metadata at INFO.
func requestResponseLogger(logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		method := c.Request.Method
		path := c.Request.URL.RequestURI()
		ip := c.ClientIP()

		logger.Infow("request received",
			"method", method,
			"path", path,
			"client_ip", ip,
		)

		c.Next()

		status := c.Writer.Status()
		latency := time.Since(start).Milliseconds()
		logger.Infow("response sent",
			"status", status,
			"latency_ms", latency,
		)
	}
}

// serve sets up Gin with conditional logging and starts the server.
func serve(cfg Configuration, logger *zap.SugaredLogger) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}

	if cfg.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	if cfg.LogLevel == "info" || cfg.LogLevel == "debug" {
		r.Use(requestResponseLogger(logger))
	}

	r.Use(gin.Recovery(), secretMiddleware(cfg.ServiceSecret, logger))
	r.GET("/", chatHandler(cfg.OpenAIKey, logger))
	return r.Run(fmt.Sprintf(":%d", cfg.Port))
}

func validateConfig(cfg Configuration) error {
	if cfg.ServiceSecret == "" {
		return fmt.Errorf("SERVICE_SECRET must be set")
	}
	if cfg.OpenAIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY must be set")
	}
	req, err := http.NewRequest(http.MethodGet, openAIModelsURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.OpenAIKey)
	client := &http.Client{Timeout: 10 * time.Second}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err == nil {
		defer resp.Body.Close()
	}
	// log validation call
	logger := zap.NewExample().Sugar()
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	logger.Infow("OpenAI key validation",
		"status", status,
		"latency_ms", latency,
	)
	if err != nil {
		return fmt.Errorf("failed to validate OPENAI_API_KEY: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OPENAI_API_KEY validation failed (status %d)", resp.StatusCode)
	}
	return nil
}

func secretMiddleware(secret string, logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Query("key") != secret {
			logger.Warnw("forbidden request", "presented_key", c.Query("key"))
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}

func chatHandler(openAIKey string, logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		prompt := c.Query("prompt")
		if prompt == "" {
			c.String(http.StatusBadRequest, "missing prompt parameter")
			return
		}

		payload := map[string]any{
			"model": "gpt-4.1",
			"messages": []map[string]string{
				{"role": "system", "content": "You are a helpful assistant."},
				{"role": "user", "content": prompt},
			},
			"temperature": 0.7,
			"max_tokens":  1024,
		}
		bodyBytes, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, openAIURL, bytes.NewReader(bodyBytes))
		req.Header.Set("Authorization", "Bearer "+openAIKey)
		req.Header.Set("Content-Type", "application/json")

		start := time.Now()
		resp, err := http.DefaultClient.Do(req)
		latency := time.Since(start).Milliseconds()
		if err != nil {
			logger.Errorw("OpenAI request error", "err", err, "latency_ms", latency)
			c.String(http.StatusBadGateway, "OpenAI request error")
			return
		}
		defer resp.Body.Close()

		respBytes, _ := io.ReadAll(resp.Body)
		content := ""
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var pr proxyResponse
			if jerr := json.Unmarshal(respBytes, &pr); jerr == nil && len(pr.Choices) > 0 {
				content = pr.Choices[0].Message.Content
			}
		}

		logger.Infow("OpenAI API response",
			"status", resp.StatusCode,
			"latency_ms", latency,
			"response_text", content,
		)

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			logger.Errorw("OpenAI API error", "status", resp.StatusCode, "body", string(respBytes))
			c.String(http.StatusBadGateway, "OpenAI API error")
			return
		}

		c.String(http.StatusOK, content)
	}
}
