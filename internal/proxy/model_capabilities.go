package proxy

import (
	"strings"
)

const (
	// defaultTemperature specifies the sampling temperature for supported models.
	defaultTemperature = 0.7
)

// --- Request Payload Structs ---
// These structs are mapped directly to the capabilities of known models.

type Reasoning struct {
	Effort string `json:"effort"`
}

// requestPayloadBase contains fields common to all requests.
type requestPayloadBase struct {
	Model           string `json:"model"`
	Input           string `json:"input"`
	MaxOutputTokens int    `json:"max_output_tokens"`
}

// requestPayloadWithTools is for models supporting tools but not temperature (e.g., gpt-5).
type requestPayloadWithTools struct {
	requestPayloadBase
	Tools      []Tool     `json:"tools,omitempty"`
	ToolChoice string     `json:"tool_choice,omitempty"`
	Reasoning  *Reasoning `json:"reasoning,omitempty"`
}

// requestPayloadWithTemperature is for models supporting temperature but not tools (e.g., gpt-4o-mini).
type requestPayloadWithTemperature struct {
	requestPayloadBase
	Temperature *float64 `json:"temperature,omitempty"`
}

// requestPayloadFull is for models supporting both temperature and tools (e.g., gpt-4o, gpt-4.1).
type requestPayloadFull struct {
	requestPayloadBase
	Temperature *float64 `json:"temperature,omitempty"`
	Tools       []Tool   `json:"tools,omitempty"`
	ToolChoice  string   `json:"tool_choice,omitempty"`
}

// Tool represents a tool available to the model.
type Tool struct {
	Type string `json:"type"`
}

// BuildRequestPayload selects the correct struct for the given model and returns it.
func BuildRequestPayload(modelIdentifier string, combinedPrompt string, webSearchEnabled bool) any {
	base := requestPayloadBase{
		Model:           modelIdentifier,
		Input:           combinedPrompt,
		MaxOutputTokens: maxOutputTokens,
	}

	// Declaratively choose the payload structure based on the model.
	switch modelIdentifier {
	case ModelNameGPT4o, ModelNameGPT41:
		p := requestPayloadFull{requestPayloadBase: base}
		temp := defaultTemperature
		p.Temperature = &temp
		if webSearchEnabled {
			p.Tools = []Tool{{Type: toolTypeWebSearch}}
			p.ToolChoice = keyAuto
		}
		return p
	case ModelNameGPT5:
		p := requestPayloadWithTools{requestPayloadBase: base}
		if webSearchEnabled {
			p.Tools = []Tool{{Type: toolTypeWebSearch}}
			p.ToolChoice = keyAuto
			p.Reasoning = &Reasoning{Effort: "medium"}
		}
		return p
	case ModelNameGPT4oMini:
		p := requestPayloadWithTemperature{requestPayloadBase: base}
		temp := defaultTemperature
		p.Temperature = &temp
		return p
	case ModelNameGPT5Mini:
		// This model has no optional parameters, so we use the base struct directly.
		return base
	default:
		// Fallback for any unknown models, assuming full capabilities as a sensible default.
		p := requestPayloadFull{requestPayloadBase: base}
		temp := defaultTemperature
		p.Temperature = &temp
		if webSearchEnabled {
			p.Tools = []Tool{{Type: toolTypeWebSearch}}
			p.ToolChoice = keyAuto
		}
		return p
	}
}

// --- Original file content below ---

// ModelPayloadSchema lists request fields allowed by a model.
type ModelPayloadSchema struct {
	// AllowedRequestFields enumerates JSON fields permitted in the request payload.
	AllowedRequestFields []string
}

const (
	// ModelNameGPT4oMini identifies the GPT-4o-mini model.
	ModelNameGPT4oMini = "gpt-4o-mini"
	// ModelNameGPT4o identifies the GPT-4o model.
	ModelNameGPT4o = "gpt-4o"
	// ModelNameGPT41 identifies the GPT-4.1 model.
	ModelNameGPT41 = "gpt-4.1"
	// ModelNameGPT5Mini identifies the GPT-5-mini model.
	ModelNameGPT5Mini = "gpt-5-mini"
	// ModelNameGPT5 identifies the GPT-5 model which does not accept the temperature field.
	ModelNameGPT5 = "gpt-5"
)

var (
	// SchemaGPT4oMini defines allowed payload fields for the GPT-4o-mini model.
	SchemaGPT4oMini = ModelPayloadSchema{AllowedRequestFields: []string{keyModel, keyInput, keyMaxOutputTokens, keyTemperature}}
	// SchemaGPT4o defines allowed payload fields for the GPT-4o model.
	SchemaGPT4o = ModelPayloadSchema{AllowedRequestFields: []string{keyModel, keyInput, keyMaxOutputTokens, keyTemperature, keyTools, keyToolChoice}}
	// SchemaGPT41 defines allowed payload fields for the GPT-4.1 model.
	SchemaGPT41 = ModelPayloadSchema{AllowedRequestFields: []string{keyModel, keyInput, keyMaxOutputTokens, keyTemperature, keyTools, keyToolChoice}}
	// SchemaGPT5Mini defines allowed payload fields for the GPT-5-mini model.
	SchemaGPT5Mini = ModelPayloadSchema{AllowedRequestFields: []string{keyModel, keyInput, keyMaxOutputTokens}}
	// SchemaGPT5 defines allowed payload fields for the GPT-5 model.
	SchemaGPT5 = ModelPayloadSchema{AllowedRequestFields: []string{keyModel, keyInput, keyMaxOutputTokens, keyTools, keyToolChoice, keyReasoning}}
)

// modelPayloadSchemas associates model identifiers with their payload schemas.
var modelPayloadSchemas = map[string]ModelPayloadSchema{
	ModelNameGPT4oMini: SchemaGPT4oMini,
	ModelNameGPT4o:     SchemaGPT4o,
	ModelNameGPT41:     SchemaGPT41,
	ModelNameGPT5Mini:  SchemaGPT5Mini,
	ModelNameGPT5:      SchemaGPT5,
}

// ResolveModelPayloadSchema returns the schema for a model or an empty schema when unknown.
func ResolveModelPayloadSchema(modelIdentifier string) ModelPayloadSchema {
	normalized := strings.ToLower(strings.TrimSpace(modelIdentifier))
	if schema, found := modelPayloadSchemas[normalized]; found {
		return schema
	}
	return ModelPayloadSchema{}
}
