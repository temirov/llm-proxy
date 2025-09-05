package proxy

import "strings"

// ModelSpecification describes the features supported by a model.
type ModelSpecification struct {
	// SupportsTemperature indicates whether the model accepts the temperature field.
	SupportsTemperature bool
	// SupportsWebSearch indicates whether the model supports web search tools.
	SupportsWebSearch bool
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

var modelSpecifications = map[string]ModelSpecification{
	ModelNameGPT4oMini: {SupportsTemperature: true},
	ModelNameGPT4o:     {SupportsTemperature: true, SupportsWebSearch: true},
	ModelNameGPT41:     {SupportsTemperature: true, SupportsWebSearch: true},
	ModelNameGPT5Mini:  {},
	ModelNameGPT5:      {SupportsTemperature: false, SupportsWebSearch: true},
}

// ResolveModelSpecification returns the specification for a model or an empty specification when unknown.
func ResolveModelSpecification(modelIdentifier string) ModelSpecification {
	normalized := strings.ToLower(strings.TrimSpace(modelIdentifier))
	if spec, found := modelSpecifications[normalized]; found {
		return spec
	}
	return ModelSpecification{}
}
