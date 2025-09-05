package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/temirov/llm-proxy/internal/constants"
	"go.uber.org/zap"
)

// ErrUnknownModel is returned when a model identifier is not recognized.
var ErrUnknownModel = errors.New(errorUnknownModel)

// capabilityCache stores capabilities retrieved from the upstream service.
type capabilityCache struct {
	cacheMutex   sync.RWMutex
	capabilities map[string]ModelCapabilities
	expiry       time.Time
	validator    *modelValidator
}

var modelCapabilityCache capabilityCache

// modelValidator caches known model identifiers from the upstream service.
type modelValidator struct {
	modelMutex sync.RWMutex
	models     map[string]struct{}
	expiry     time.Time
	apiKey     string
	logger     *zap.SugaredLogger
}

// set replaces the capability cache with the provided map and updates the expiry.
func (cache *capabilityCache) set(newCapabilities map[string]ModelCapabilities) {
	cache.cacheMutex.Lock()
	cache.capabilities = newCapabilities
	cache.expiry = time.Now().Add(capabilitiesCacheTTL)
	cache.cacheMutex.Unlock()
}

// get retrieves capabilities for the supplied model identifier, refreshing if expired.
func (cache *capabilityCache) get(modelIdentifier string) (ModelCapabilities, bool) {
	cache.cacheMutex.RLock()
	capability, found := cache.capabilities[modelIdentifier]
	expired := time.Now().After(cache.expiry)
	cache.cacheMutex.RUnlock()
	if expired && cache.validator != nil {
		if refreshError := cache.validator.refresh(); refreshError == nil {
			cache.cacheMutex.RLock()
			capability, found = cache.capabilities[modelIdentifier]
			cache.cacheMutex.RUnlock()
		}
	}
	return capability, found
}

// newModelValidator creates a modelValidator and loads the initial model list.
func newModelValidator(openAIKey string, structuredLogger *zap.SugaredLogger) (*modelValidator, error) {
	validator := &modelValidator{apiKey: openAIKey, logger: structuredLogger}
	modelCapabilityCache.validator = validator
	if refreshError := validator.refresh(); refreshError != nil {
		return nil, refreshError
	}
	return validator, nil
}

// refresh retrieves the model list from OpenAI and updates the cache.
func (validator *modelValidator) refresh() error {
	httpRequest, requestError := http.NewRequest(http.MethodGet, modelsURL, nil)
	if requestError != nil {
		return requestError
	}
	httpRequest.Header.Set(headerAuthorization, headerAuthorizationPrefix+validator.apiKey)

	startTime := time.Now()
	httpResponse, httpError := HTTPClient.Do(httpRequest)
	latencyMillis := time.Since(startTime).Milliseconds()
	if httpError != nil {
		validator.logger.Errorw(
			logEventOpenAIModelsListError,
			constants.LogFieldError,
			httpError,
			constants.LogFieldLatencyMilliseconds,
			latencyMillis,
		)
		return httpError
	}
	defer httpResponse.Body.Close()

	validator.logger.Infow(logEventOpenAIModelsList, logFieldHTTPStatus, httpResponse.StatusCode, constants.LogFieldLatencyMilliseconds, latencyMillis)
	if httpResponse.StatusCode != http.StatusOK {
		bodyBytes, readError := io.ReadAll(httpResponse.Body)
		if readError != nil {
			return fmt.Errorf("%s: status=%d", logEventOpenAIModelsListError, httpResponse.StatusCode)
		}
		return fmt.Errorf("%s: status=%d body=%s", logEventOpenAIModelsListError, httpResponse.StatusCode, string(bodyBytes))
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if decodeError := json.NewDecoder(httpResponse.Body).Decode(&payload); decodeError != nil {
		return decodeError
	}
	modelSet := make(map[string]struct{}, len(payload.Data))
	capabilityMap := make(map[string]ModelCapabilities, len(payload.Data))
	for _, modelEntry := range payload.Data {
		modelSet[modelEntry.ID] = struct{}{}
		if capabilities, fetchError := fetchModelCapabilities(modelEntry.ID, validator.apiKey); fetchError == nil {
			capabilityMap[modelEntry.ID] = capabilities
		} else {
			validator.logger.Debugw(logEventOpenAIModelCapabilitiesError, constants.LogFieldError, fetchError)
		}
	}
	modelCapabilityCache.set(capabilityMap)
	validator.modelMutex.Lock()
	validator.models = modelSet
	validator.expiry = time.Now().Add(modelsCacheTTL)
	validator.modelMutex.Unlock()
	return nil
}

// Verify checks whether the provided model identifier is known.
func (validator *modelValidator) Verify(modelIdentifier string) error {
	validator.modelMutex.RLock()
	currentExpiry := validator.expiry
	_, known := validator.models[modelIdentifier]
	validator.modelMutex.RUnlock()

	if time.Now().After(currentExpiry) || validator.models == nil {
		if refreshError := validator.refresh(); refreshError != nil {
			return errors.New(errorOpenAIModelValidation)
		}
		validator.modelMutex.RLock()
		_, known = validator.models[modelIdentifier]
		validator.modelMutex.RUnlock()
	}
	if !known {
		return fmt.Errorf("%w: %s", ErrUnknownModel, modelIdentifier)
	}
	return nil
}
