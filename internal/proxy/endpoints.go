package proxy

const (
	defaultResponsesURL = "https://api.openai.com/v1/responses"
	defaultModelsURL    = "https://api.openai.com/v1/models"
)

// Endpoints holds the URLs for the OpenAI responses and models endpoints.
type Endpoints struct {
	ResponsesURL string
	ModelsURL    string
}

// NewEndpoints returns an Endpoints instance initialized with default URLs.
func NewEndpoints() *Endpoints {
	return &Endpoints{
		ResponsesURL: defaultResponsesURL,
		ModelsURL:    defaultModelsURL,
	}
}

// DefaultEndpoints provides the endpoint URLs used by the proxy.
var DefaultEndpoints = NewEndpoints()

// GetResponsesURL returns the URL used for the OpenAI responses endpoint.
func (endpointConfiguration *Endpoints) GetResponsesURL() string {
	return endpointConfiguration.ResponsesURL
}

// SetResponsesURL sets the URL for the OpenAI responses endpoint.
func (endpointConfiguration *Endpoints) SetResponsesURL(newURL string) {
	endpointConfiguration.ResponsesURL = newURL
}

// ResetResponsesURL resets the responses endpoint to the default.
func (endpointConfiguration *Endpoints) ResetResponsesURL() {
	endpointConfiguration.ResponsesURL = defaultResponsesURL
}

// GetModelsURL returns the URL used for the OpenAI models endpoint.
func (endpointConfiguration *Endpoints) GetModelsURL() string {
	return endpointConfiguration.ModelsURL
}

// SetModelsURL sets the URL for the OpenAI models endpoint.
func (endpointConfiguration *Endpoints) SetModelsURL(newURL string) {
	endpointConfiguration.ModelsURL = newURL
}

// ResetModelsURL resets the models endpoint to the default.
func (endpointConfiguration *Endpoints) ResetModelsURL() {
	endpointConfiguration.ModelsURL = defaultModelsURL
}
