package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

const (
	// apiFlavorResponses identifies the responses API flavor.
	apiFlavorResponses = "responses"
	// modelPrefixGPT4oMini is the prefix for GPT-4o-mini models.
	modelPrefixGPT4oMini = "gpt-4o-mini"
	// modelPrefixGPT4o is the prefix for GPT-4o models.
	modelPrefixGPT4o = "gpt-4o"
	// modelPrefixGPT41 is the prefix for GPT-4.1 models.
	modelPrefixGPT41 = "gpt-4.1"
	// modelPrefixGPT5Mini is the prefix for GPT-5-mini models.
	modelPrefixGPT5Mini = "gpt-5-mini"
	// modelPrefixGPT5 is the prefix for GPT-5 models.
	modelPrefixGPT5 = "gpt-5"
	// modelNameSeparator divides model prefix and variant.
	modelNameSeparator = "-"
)

// ModelCapabilities describes the features supported by a model.
type ModelCapabilities struct {
	apiFlavor            string
	allowedRequestFields map[string]struct{}
}

// SupportsField reports whether the capability set permits the specified request field.
func (capabilities ModelCapabilities) SupportsField(fieldName string) bool {
	_, allowed := capabilities.allowedRequestFields[fieldName]
	return allowed
}

// SupportsWebSearch reports whether the model allows web search.
func (capabilities ModelCapabilities) SupportsWebSearch() bool {
	return capabilities.SupportsField(keyTools)
}

// SupportsTemperature reports whether the model allows setting temperature.
func (capabilities ModelCapabilities) SupportsTemperature() bool {
	return capabilities.SupportsField(keyTemperature)
}

// capabilityCache holds capabilities retrieved from the upstream service.
var (
	capabilityCache      = make(map[string]ModelCapabilities)
	capabilityCacheMutex sync.RWMutex
)

// setCapabilityCache replaces the capability cache with the provided map.
func setCapabilityCache(newCache map[string]ModelCapabilities) {
	capabilityCacheMutex.Lock()
	capabilityCache = newCache
	capabilityCacheMutex.Unlock()
}

// cachedCapabilities retrieves capabilities for the supplied model identifier.
func cachedCapabilities(modelIdentifier string) (ModelCapabilities, bool) {
	capabilityCacheMutex.RLock()
	capabilities, found := capabilityCache[modelIdentifier]
	capabilityCacheMutex.RUnlock()
	return capabilities, found
}

// fetchModelCapabilities retrieves the capability description for a model from the upstream service.
func fetchModelCapabilities(modelIdentifier string, openAIKey string) (ModelCapabilities, error) {
	resourceURL := ModelsURL() + "/" + modelIdentifier
	httpRequest, requestError := http.NewRequest(http.MethodGet, resourceURL, nil)
	if requestError != nil {
		return ModelCapabilities{}, requestError
	}
	httpRequest.Header.Set(headerAuthorization, headerAuthorizationPrefix+openAIKey)

	httpResponse, httpError := HTTPClient.Do(httpRequest)
	if httpError != nil {
		return ModelCapabilities{}, httpError
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(httpResponse.Body)
		return ModelCapabilities{}, fmt.Errorf("status=%d body=%s", httpResponse.StatusCode, string(bodyBytes))
	}

	var payload map[string]any
	if decodeError := json.NewDecoder(httpResponse.Body).Decode(&payload); decodeError != nil {
		return ModelCapabilities{}, decodeError
	}

	rawFields, _ := payload[jsonFieldAllowedRequestFields].([]any)
	allowed := make(map[string]struct{}, len(rawFields))
	for _, field := range rawFields {
		if fieldName, ok := field.(string); ok {
			allowed[fieldName] = struct{}{}
		}
	}
	return ModelCapabilities{apiFlavor: apiFlavorResponses, allowedRequestFields: allowed}, nil
}

// capabilitiesByPrefix defines known capabilities for recognized model prefixes.
var capabilitiesByPrefix = map[string]ModelCapabilities{
	modelPrefixGPT4oMini: {apiFlavor: apiFlavorResponses, allowedRequestFields: map[string]struct{}{keyTemperature: {}}},
	modelPrefixGPT4o:     {apiFlavor: apiFlavorResponses, allowedRequestFields: map[string]struct{}{keyTemperature: {}, keyTools: {}}},
	modelPrefixGPT41:     {apiFlavor: apiFlavorResponses, allowedRequestFields: map[string]struct{}{keyTemperature: {}, keyTools: {}}},
	modelPrefixGPT5Mini:  {apiFlavor: apiFlavorResponses, allowedRequestFields: map[string]struct{}{}},
	modelPrefixGPT5:      {apiFlavor: apiFlavorResponses, allowedRequestFields: map[string]struct{}{keyTemperature: {}, keyTools: {}}},
}

// lookupModelCapabilities finds capabilities for the given model identifier.
func lookupModelCapabilities(modelIdentifier string) (ModelCapabilities, bool) {
	for {
		if capabilities, found := capabilitiesByPrefix[modelIdentifier]; found {
			return capabilities, true
		}
		lastSeparatorIndex := strings.LastIndex(modelIdentifier, modelNameSeparator)
		if lastSeparatorIndex == -1 {
			break
		}
		modelIdentifier = modelIdentifier[:lastSeparatorIndex]
	}
	return ModelCapabilities{}, false
}

// ResolveModelSpecification returns capabilities using the capability cache or static table.
func ResolveModelSpecification(modelIdentifier string) ModelCapabilities {
	lower := strings.ToLower(strings.TrimSpace(modelIdentifier))
	if cached, found := cachedCapabilities(lower); found {
		return cached
	}
	if capabilities, found := lookupModelCapabilities(lower); found {
		return capabilities
	}
	return ModelCapabilities{apiFlavor: apiFlavorResponses, allowedRequestFields: map[string]struct{}{}}
}
