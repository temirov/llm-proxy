package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/temirov/llm-proxy/internal/logging"
	"go.uber.org/zap"
)

type modelValidator struct {
	mu     sync.RWMutex
	models map[string]struct{}
	expiry time.Time
	apiKey string
	logger *zap.SugaredLogger
}

func newModelValidator(openAIKey string, structuredLogger *zap.SugaredLogger) (*modelValidator, error) {
	validator := &modelValidator{apiKey: openAIKey, logger: structuredLogger}
	if err := validator.refresh(); err != nil {
		return nil, err
	}
	return validator, nil
}

func (validator *modelValidator) refresh() error {
	httpRequest, requestError := http.NewRequest(http.MethodGet, ModelsURL, nil)
	if requestError != nil {
		return requestError
	}
	httpRequest.Header.Set(headerAuthorization, headerAuthorizationPrefix+validator.apiKey)

	startTime := time.Now()
	httpResponse, httpError := HTTPClient.Do(httpRequest)
	latencyMilliseconds := time.Since(startTime).Milliseconds()
	if httpError != nil {
		validator.logger.Errorw(logEventOpenAIModelsListError, "err", httpError, logging.LogFieldLatencyMilliseconds, latencyMilliseconds)
		return httpError
	}
	defer httpResponse.Body.Close()

	validator.logger.Infow(logEventOpenAIModelsList, logFieldHTTPStatus, httpResponse.StatusCode, logging.LogFieldLatencyMilliseconds, latencyMilliseconds)
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
	validator.mu.Lock()
	validator.models = modelSet
	validator.expiry = time.Now().Add(modelsCacheTTL)
	validator.mu.Unlock()
	return nil
}

func (validator *modelValidator) Verify(modelIdentifier string) error {
	validator.mu.RLock()
	currentExpiry := validator.expiry
	_, known := validator.models[modelIdentifier]
	validator.mu.RUnlock()

	if time.Now().After(currentExpiry) || validator.models == nil {
		if err := validator.refresh(); err != nil {
			return errors.New(errorOpenAIModelValidation)
		}
		validator.mu.RLock()
		_, known = validator.models[modelIdentifier]
		validator.mu.RUnlock()
	}
	if !known {
		return fmt.Errorf("unknown model: %s", modelIdentifier)
	}
	return nil
}
