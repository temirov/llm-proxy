package proxy

import "strings"

// ModelPayloadSchema enumerates the request fields accepted by a model.
type ModelPayloadSchema struct {
	// Temperature indicates whether the model accepts the temperature field.
	Temperature bool
	// Tools indicates whether the model accepts the tools field.
	Tools bool
	// ToolChoice indicates whether the model accepts the tool_choice field.
	ToolChoice bool
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
	SchemaGPT4oMini = ModelPayloadSchema{Temperature: true}
	// SchemaGPT4o defines allowed payload fields for the GPT-4o model.
	SchemaGPT4o = ModelPayloadSchema{Temperature: true, Tools: true, ToolChoice: true}
	// SchemaGPT41 defines allowed payload fields for the GPT-4.1 model.
	SchemaGPT41 = ModelPayloadSchema{Temperature: true, Tools: true, ToolChoice: true}
	// SchemaGPT5Mini defines allowed payload fields for the GPT-5-mini model.
	SchemaGPT5Mini = ModelPayloadSchema{}
	// SchemaGPT5 defines allowed payload fields for the GPT-5 model.
	SchemaGPT5 = ModelPayloadSchema{Tools: true, ToolChoice: true}
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
