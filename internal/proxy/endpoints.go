package proxy

const (
	defaultResponsesURL = "https://api.openai.com/v1/responses"
	defaultModelsURL    = "https://api.openai.com/v1/models"
)

var (
	responsesURL = defaultResponsesURL
	modelsURL    = defaultModelsURL
)

// ResponsesURL returns the URL used for the OpenAI responses endpoint.
func ResponsesURL() string { return responsesURL }

// SetResponsesURL sets the URL for the OpenAI responses endpoint.
func SetResponsesURL(newURL string) { responsesURL = newURL }

// ResetResponsesURL resets the responses endpoint to the default.
func ResetResponsesURL() { responsesURL = defaultResponsesURL }

// ModelsURL returns the URL used for the OpenAI models endpoint.
func ModelsURL() string { return modelsURL }

// SetModelsURL sets the URL for the OpenAI models endpoint.
func SetModelsURL(newURL string) { modelsURL = newURL }

// ResetModelsURL resets the models endpoint to the default.
func ResetModelsURL() { modelsURL = defaultModelsURL }
