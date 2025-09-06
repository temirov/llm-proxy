package proxy

import "sync"

const (
	defaultResponsesURL = "https://api.openai.com/v1/responses"
	defaultModelsURL    = "https://api.openai.com/v1/models"
)

// Endpoints holds the URLs for the OpenAI responses and models endpoints.
// Access to the URLs is guarded by a read–write mutex to ensure safe
// concurrent reads and writes.
type Endpoints struct {
	responsesURL string
	modelsURL    string
	accessMutex  sync.RWMutex
}

// NewEndpoints returns an Endpoints instance initialized with default URLs.
func NewEndpoints() *Endpoints {
	return &Endpoints{
		responsesURL: defaultResponsesURL,
		modelsURL:    defaultModelsURL,
	}
}

// GetResponsesURL returns the URL used for the OpenAI responses endpoint.
func (endpointConfiguration *Endpoints) GetResponsesURL() string {
	endpointConfiguration.accessMutex.RLock()
	defer endpointConfiguration.accessMutex.RUnlock()
	return endpointConfiguration.responsesURL
}

// SetResponsesURL sets the URL for the OpenAI responses endpoint.
func (endpointConfiguration *Endpoints) SetResponsesURL(newURL string) {
	endpointConfiguration.accessMutex.Lock()
	defer endpointConfiguration.accessMutex.Unlock()
	endpointConfiguration.responsesURL = newURL
}

// ResetResponsesURL resets the responses endpoint to the default.
func (endpointConfiguration *Endpoints) ResetResponsesURL() {
	endpointConfiguration.accessMutex.Lock()
	defer endpointConfiguration.accessMutex.Unlock()
	endpointConfiguration.responsesURL = defaultResponsesURL
}

// GetModelsURL returns the URL used for the OpenAI models endpoint.
func (endpointConfiguration *Endpoints) GetModelsURL() string {
	endpointConfiguration.accessMutex.RLock()
	defer endpointConfiguration.accessMutex.RUnlock()
	return endpointConfiguration.modelsURL
}

// SetModelsURL sets the URL for the OpenAI models endpoint.
func (endpointConfiguration *Endpoints) SetModelsURL(newURL string) {
	endpointConfiguration.accessMutex.Lock()
	defer endpointConfiguration.accessMutex.Unlock()
	endpointConfiguration.modelsURL = newURL
}

// ResetModelsURL resets the models endpoint to the default.
func (endpointConfiguration *Endpoints) ResetModelsURL() {
	endpointConfiguration.accessMutex.Lock()
	defer endpointConfiguration.accessMutex.Unlock()
	endpointConfiguration.modelsURL = defaultModelsURL
}
