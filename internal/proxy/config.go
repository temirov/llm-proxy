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
	DefaultModel = ModelNameGPT41

	DefaultRequestTimeoutSeconds      = 180 // overall app-side request timeout
	DefaultUpstreamPollTimeoutSeconds = 60  // poll budget after "incomplete"
	DefaultMaxOutputTokens            = 1024
)

// Configuration holds runtime settings.
type Configuration struct {
	ServiceSecret              string
	OpenAIKey                  string
	Port                       int
	LogLevel                   string
	SystemPrompt               string
	WorkerCount                int
	QueueSize                  int
	RequestTimeoutSeconds      int
	UpstreamPollTimeoutSeconds int
	MaxOutputTokens            int
}

// validateConfig confirms required settings are present.
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

// ErrUpstreamIncomplete indicates that the upstream provider returned an incomplete response before the poll deadline.
var ErrUpstreamIncomplete = errors.New(errorUpstreamIncomplete)
