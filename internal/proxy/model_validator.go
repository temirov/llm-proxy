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

// modelValidator caches known model identifiers from the upstream service.
type modelValidator struct {
	// modelMutex guards access to models and expiry.
	modelMutex sync.RWMutex
	models     map[string]struct{}
	expiry     time.Time
	apiKey     string
	logger     *zap.SugaredLogger
}

// newModelValidator creates a modelValidator and loads the initial model list.
func newModelValidator(openAIKey string, structuredLogger *zap.SugaredLogger) (*modelValidator, error) {
	validator := &modelValidator{apiKey: openAIKey, logger: structuredLogger}
	if err := validator.refresh(); err != nil {
		return nil, err
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
		validator.logger.Errorw(logEventOpenAIModelsListError, "err", httpError, constants.LogFieldLatencyMilliseconds, latencyMillis)
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
	if err := json.NewDecoder(httpResponse.Body).Decode(&payload); err != nil {
		return err
	}
	modelSet := make(map[string]struct{}, len(payload.Data))
	for _, modelEntry := range payload.Data {
		modelSet[modelEntry.ID] = struct{}{}
	}
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
		if err := validator.refresh(); err != nil {
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
