package proxy

import "strings"

const (
	apiResponses = "responses"
)

type modelSpecification struct {
	API                   string
	IncludeTemperature    bool
	IncludeWebSearchTools bool
}

func resolveModelSpecification(modelIdentifier string) modelSpecification {
	lower := strings.ToLower(strings.TrimSpace(modelIdentifier))

	switch {
	case strings.HasPrefix(lower, "gpt-4.1"):
		return modelSpecification{API: apiResponses, IncludeTemperature: true, IncludeWebSearchTools: true}
	case strings.HasPrefix(lower, "gpt-4o"):
		return modelSpecification{API: apiResponses, IncludeTemperature: true, IncludeWebSearchTools: true}
	case strings.HasPrefix(lower, "gpt-5-mini"):
		return modelSpecification{API: apiResponses, IncludeTemperature: false, IncludeWebSearchTools: false}
	default:
		return modelSpecification{API: apiResponses, IncludeTemperature: false, IncludeWebSearchTools: false}
	}
}
