package proxy

import (
	"errors"
	"strings"
	"time"

	"github.com/temirov/llm-proxy/internal/apperrors"
)

const (
	// DefaultPort is the TCP port used by the HTTP server when no explicit port is provided.
	DefaultPort = 8080
	// DefaultWorkers is the number of worker goroutines that process upstream requests.
	DefaultWorkers = 4
	// DefaultQueueSize is the capacity of the internal request queue.
	DefaultQueueSize = 100
	// DefaultModel is the model identifier used when the client does not supply one.
	DefaultModel = "gpt-4.1"

	modelsCacheTTL = 24 * time.Hour
)

// Configuration captures runtime settings for the HTTP server and upstream requests.
type Configuration struct {
	ServiceSecret string
	OpenAIKey     string
	Port          int
	LogLevel      string
	SystemPrompt  string
	WorkerCount   int
	QueueSize     int
}

// validateConfig confirms the presence of required configuration values.
func validateConfig(config Configuration) error {
	if strings.TrimSpace(config.ServiceSecret) == "" {
		return apperrors.ErrMissingServiceSecret
	}
	if strings.TrimSpace(config.OpenAIKey) == "" {
		return apperrors.ErrMissingOpenAIKey
	}
	return nil
}

var requestTimeout = 30 * time.Second

var ErrUpstreamIncomplete = errors.New("OpenAI API error (incomplete response)")
