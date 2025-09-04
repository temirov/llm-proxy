package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/temirov/llm-proxy/internal/constants"
	"github.com/temirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

// HTTPDoer executes HTTP requests, allowing the proxy to abstract the underlying HTTP client.
type HTTPDoer interface {
	Do(httpRequest *http.Request) (*http.Response, error)
}

var (
	// HTTPClient is the default HTTPDoer implementation that delegates to http.DefaultClient.
	HTTPClient          HTTPDoer = http.DefaultClient
	maxOutputTokens              = DefaultMaxOutputTokens
	upstreamPollTimeout time.Duration
)

// UpstreamPollTimeout returns the current upstream poll timeout.
func UpstreamPollTimeout() time.Duration { return upstreamPollTimeout }

// SetUpstreamPollTimeout overrides the upstream poll timeout value.
func SetUpstreamPollTimeout(newTimeout time.Duration) { upstreamPollTimeout = newTimeout }

type responsesAPIShim struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Tool represents a tool available to the model.
type Tool struct {
	Type string `json:"type"`
}

// OpenAIRequest defines the payload for the OpenAI responses API.
type OpenAIRequest struct {
	// Model is the model identifier.
	Model string `json:"model"`
	// Input is the list of messages forming the conversation.
	Input []map[string]string `json:"input"`
	// MaxOutputTokens limits the number of tokens generated in the response.
	MaxOutputTokens int `json:"max_output_tokens"`
	// Temperature adjusts the randomness of the response.
	Temperature *float64 `json:"temperature,omitempty"`
	// Tools lists the tools available to the model.
	Tools []Tool `json:"tools,omitempty"`
	// ToolChoice selects a tool to use for the request.
	ToolChoice string `json:"tool_choice,omitempty"`
}

const (
	// unsupportedTemperatureParameterToken marks an error response mentioning the temperature parameter.
	unsupportedTemperatureParameterToken = "'temperature'"
	// unsupportedToolsParameterToken marks an error response mentioning the tools parameter.
	unsupportedToolsParameterToken = "'tools'"
)

const lineBreak = "\n"

// openAIRequest sends a prompt to the OpenAI responses API and returns the resulting text.
// It retries without unsupported parameters and polls for completion when needed.
func openAIRequest(openAIKey string, modelIdentifier string, userPrompt string, systemPrompt string, webSearchEnabled bool, structuredLogger *zap.SugaredLogger) (string, error) {
	messageList := []map[string]string{
		{keyRole: keySystem, keyContent: systemPrompt},
		{keyRole: keyUser, keyContent: userPrompt},
	}

	modelCapabilities := ResolveModelSpecification(modelIdentifier)

	requestPayload := OpenAIRequest{
		Model:           modelIdentifier,
		Input:           messageList,
		MaxOutputTokens: maxOutputTokens,
	}
	if modelCapabilities.SupportsTemperature() {
		temperature := 0.7
		requestPayload.Temperature = &temperature
	}
	if webSearchEnabled && modelCapabilities.SupportsWebSearch() {
		requestPayload.Tools = []Tool{{Type: toolTypeWebSearch}}
		requestPayload.ToolChoice = keyAuto
	}

	payloadBytes, marshalError := json.Marshal(requestPayload)
	if marshalError != nil {
		structuredLogger.Errorw(logEventMarshalRequestPayload, constants.LogFieldError, marshalError)
		return "", errors.New(errorRequestBuild)
	}

	requestContext, cancelRequest := context.WithTimeout(context.Background(), requestTimeout)
	defer cancelRequest()
	httpRequest, buildError := buildAuthorizedJSONRequest(requestContext, http.MethodPost, responsesURL, openAIKey, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return "", errors.New(errorRequestBuild)
	}

	statusCode, responseBytes, latencyMillis, requestError := performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIRequestError)
	if requestError != nil {
		if errors.Is(requestError, context.DeadlineExceeded) {
			return "", requestError
		}
		return "", errors.New(errorOpenAIRequest)
	}

	if statusCode >= http.StatusBadRequest &&
		bytes.Contains(responseBytes, []byte(unsupportedTemperatureParameterToken)) &&
		requestPayload.Temperature != nil {
		structuredLogger.Infow(logEventRetryingWithoutParam, "parameter", keyTemperature)
		requestPayload.Temperature = nil
		retryPayloadBytes, marshalRetryError := json.Marshal(requestPayload)
		if marshalRetryError != nil {
			structuredLogger.Errorw(logEventMarshalRequestPayload, constants.LogFieldError, marshalRetryError)
			return "", errors.New(errorRequestBuild)
		}
		retryContext, cancelRetry := context.WithTimeout(context.Background(), requestTimeout)
		defer cancelRetry()
		retryRequest, buildRetryError := buildAuthorizedJSONRequest(retryContext, http.MethodPost, responsesURL, openAIKey, bytes.NewReader(retryPayloadBytes))
		if buildRetryError != nil {
			structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildRetryError)
			return "", errors.New(errorRequestBuild)
		}
		statusCode, responseBytes, latencyMillis, requestError = performResponsesRequest(retryRequest, structuredLogger, logEventOpenAIRequestError)
		if requestError != nil {
			if errors.Is(requestError, context.DeadlineExceeded) {
				return "", requestError
			}
			return "", errors.New(errorOpenAIRequest)
		}
	}

	if statusCode >= http.StatusBadRequest &&
		bytes.Contains(responseBytes, []byte(unsupportedToolsParameterToken)) &&
		len(requestPayload.Tools) > 0 {
		structuredLogger.Infow(logEventRetryingWithoutParam, "parameter", keyTools)
		requestPayload.Tools = nil
		requestPayload.ToolChoice = ""
		retryPayloadBytes, marshalRetryError := json.Marshal(requestPayload)
		if marshalRetryError != nil {
			structuredLogger.Errorw(logEventMarshalRequestPayload, constants.LogFieldError, marshalRetryError)
			return "", errors.New(errorRequestBuild)
		}
		retryContext, cancelRetry := context.WithTimeout(context.Background(), requestTimeout)
		defer cancelRetry()
		retryRequest, buildRetryError := buildAuthorizedJSONRequest(retryContext, http.MethodPost, responsesURL, openAIKey, bytes.NewReader(retryPayloadBytes))
		if buildRetryError != nil {
			structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildRetryError)
			return "", errors.New(errorRequestBuild)
		}
		statusCode, responseBytes, latencyMillis, requestError = performResponsesRequest(retryRequest, structuredLogger, logEventOpenAIRequestError)
		if requestError != nil {
			if errors.Is(requestError, context.DeadlineExceeded) {
				return "", requestError
			}
			return "", errors.New(errorOpenAIRequest)
		}
	}

	var decodedObject map[string]any
	if unmarshalError := json.Unmarshal(responseBytes, &decodedObject); unmarshalError != nil {
		structuredLogger.Errorw(logEventParseOpenAIResponseFailed, constants.LogFieldError, unmarshalError)
		decodedObject = nil
	}

	outputText := extractTextFromAny(decodedObject, responseBytes)
	responseIdentifier := getString(decodedObject, jsonFieldID)
	apiStatus := strings.ToLower(getString(decodedObject, jsonFieldStatus))

	structuredLogger.Infow(
		logEventOpenAIResponse,
		logFieldHTTPStatus, statusCode,
		logFieldAPIStatus, apiStatus,
		constants.LogFieldLatencyMilliseconds, latencyMillis,
		logFieldResponseText, outputText,
	)

	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		structuredLogger.Desugar().Error(
			errorOpenAIAPI,
			zap.Int(logFieldStatus, statusCode),
			zap.ByteString(logFieldResponseBody, responseBytes),
		)
		return "", errors.New(errorOpenAIAPI)
	}

	if utils.IsBlank(outputText) && !utils.IsBlank(responseIdentifier) {
		finalText, pollError := pollResponseUntilDone(openAIKey, responseIdentifier, structuredLogger)
		if pollError != nil {
			structuredLogger.Errorw(
				logEventOpenAIPollError,
				"id",
				responseIdentifier,
				constants.LogFieldError,
				pollError,
			)
			return "", errors.New(errorOpenAIAPI)
		}
		if utils.IsBlank(finalText) {
			structuredLogger.Desugar().Error(
				errorOpenAIAPI,
				zap.Int(logFieldStatus, statusCode),
				zap.ByteString(logFieldResponseBody, responseBytes),
			)
			return "", errors.New(errorOpenAIAPINoText)
		}
		return finalText, nil
	}

	if utils.IsBlank(outputText) {
		structuredLogger.Desugar().Error(
			errorOpenAIAPI,
			zap.Int(logFieldStatus, statusCode),
			zap.ByteString(logFieldResponseBody, responseBytes),
		)
		return "", errors.New(errorOpenAIAPI)
	}
	return outputText, nil
}

// pollResponseUntilDone repeatedly fetches a response until it is complete or the poll timeout elapses.
func pollResponseUntilDone(openAIKey string, responseIdentifier string, structuredLogger *zap.SugaredLogger) (string, error) {
	deadlineInstant := time.Now().Add(upstreamPollTimeout)
	pollInterval := 300 * time.Millisecond

	for {
		if time.Now().After(deadlineInstant) {
			return "", ErrUpstreamIncomplete
		}
		pollContext, cancelPoll := context.WithDeadline(context.Background(), deadlineInstant)
		textCandidate, responseComplete, fetchError := fetchResponseByID(pollContext, openAIKey, responseIdentifier, structuredLogger)
		cancelPoll()
		if fetchError != nil {
			return "", fetchError
		}
		if responseComplete && !utils.IsBlank(textCandidate) {
			return textCandidate, nil
		}
		if responseComplete {
			return "", ErrUpstreamIncomplete
		}
		time.Sleep(pollInterval)
	}
}

// fetchResponseByID retrieves a response by identifier and reports whether the response is complete.
func fetchResponseByID(contextToUse context.Context, openAIKey string, responseIdentifier string, structuredLogger *zap.SugaredLogger) (string, bool, error) {
	resourceURL := responsesURL + "/" + responseIdentifier
	requestContext, cancelRequest := context.WithTimeout(contextToUse, requestTimeout)
	defer cancelRequest()
	httpRequest, buildError := buildAuthorizedJSONRequest(requestContext, http.MethodGet, resourceURL, openAIKey, nil)
	if buildError != nil {
		return "", false, buildError
	}

	_, responseBytes, _, requestError := performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIPollError)
	if requestError != nil {
		if errors.Is(requestError, context.DeadlineExceeded) {
			return "", false, requestError
		}
		return "", false, errors.New(errorOpenAIRequest)
	}

	var decodedObject map[string]any
	if unmarshalError := json.Unmarshal(responseBytes, &decodedObject); unmarshalError != nil {
		structuredLogger.Errorw(logEventParseOpenAIResponseFailed, constants.LogFieldError, unmarshalError)
		decodedObject = nil
	}
	responseStatus := strings.ToLower(getString(decodedObject, jsonFieldStatus))
	outputText := extractTextFromAny(decodedObject, responseBytes)

	switch responseStatus {
	case statusCompleted, statusSucceeded, statusDone:
		return outputText, true, nil
	case statusCancelled, statusFailed, statusErrored:
		return "", true, errors.New(errorOpenAIFailedStatus)
	default:
		return "", false, nil
	}
}

// getString returns a string value from the provided container for the specified field.
func getString(container map[string]any, field string) string {
	if container == nil {
		return ""
	}
	if rawValue, present := container[field]; present {
		if castValue, isString := rawValue.(string); isString {
			return castValue
		}
	}
	return ""
}

// extractTextFromAny obtains text content from known response shapes using a single unmarshal pass.
func extractTextFromAny(container map[string]any, rawPayload []byte) string {
	if container != nil {
		if direct, isString := container[jsonFieldOutputText].(string); isString && !utils.IsBlank(direct) {
			return direct
		}
	}

	type contentPart struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type contentContainer struct {
		Content []contentPart `json:"content"`
	}
	var envelope struct {
		Output   json.RawMessage `json:"output"`
		Response json.RawMessage `json:"response"`
		responsesAPIShim
	}
	if unmarshalError := json.Unmarshal(rawPayload, &envelope); unmarshalError != nil {
		return ""
	}

	if len(envelope.Output) > 0 {
		var outputItems []contentContainer
		if unmarshalError := json.Unmarshal(envelope.Output, &outputItems); unmarshalError == nil && len(outputItems) > 0 {
			var textBuilder strings.Builder
			for _, outputEntry := range outputItems {
				for _, contentEntry := range outputEntry.Content {
					if !utils.IsBlank(contentEntry.Text) {
						if textBuilder.Len() > 0 {
							textBuilder.WriteString(lineBreak)
						}
						textBuilder.WriteString(contentEntry.Text)
					}
				}
			}
			if textBuilder.Len() > 0 {
				return textBuilder.String()
			}
		}
	}

	if len(envelope.Response) > 0 {
		var responseItems []contentContainer
		if unmarshalError := json.Unmarshal(envelope.Response, &responseItems); unmarshalError == nil && len(responseItems) > 0 {
			var textBuilder strings.Builder
			for _, responseEntry := range responseItems {
				for _, contentEntry := range responseEntry.Content {
					if !utils.IsBlank(contentEntry.Text) {
						if textBuilder.Len() > 0 {
							textBuilder.WriteString(lineBreak)
						}
						textBuilder.WriteString(contentEntry.Text)
					}
				}
			}
			if textBuilder.Len() > 0 {
				return textBuilder.String()
			}
		}
	}

	if len(envelope.Choices) > 0 {
		return envelope.Choices[0].Message.Content
	}
	return ""
}

// performResponsesRequest executes the HTTP request and retries when the status code indicates a server error.
// The retries continue with exponential backoff until the request context deadline is exceeded.
func performResponsesRequest(httpRequest *http.Request, structuredLogger *zap.SugaredLogger, logEvent string) (int, []byte, int64, error) {
	var statusCode int
	var responseBytes []byte
	var latencyMillis int64
	operation := func() error {
		var transportError error
		statusCode, responseBytes, latencyMillis, transportError = utils.PerformHTTPRequest(HTTPClient.Do, httpRequest, structuredLogger, logEvent)
		if transportError != nil {
			return transportError
		}
		if statusCode >= http.StatusInternalServerError {
			return errors.New(errorOpenAIAPI)
		}
		return nil
	}
	retryStrategy := backoff.NewExponentialBackOff()
	retryError := backoff.Retry(operation, backoff.WithContext(retryStrategy, httpRequest.Context()))
	return statusCode, responseBytes, latencyMillis, retryError
}

// buildAuthorizedJSONRequest constructs an HTTP request with authorization and JSON content type headers using the provided context.
func buildAuthorizedJSONRequest(contextToUse context.Context, method string, resourceURL string, openAIKey string, body io.Reader) (*http.Request, error) {
	httpRequest, httpRequestError := http.NewRequestWithContext(contextToUse, method, resourceURL, body)
	if httpRequestError != nil {
		return nil, httpRequestError
	}
	httpRequest.Header.Set(headerAuthorization, headerAuthorizationPrefix+openAIKey)
	httpRequest.Header.Set(headerContentType, mimeApplicationJSON)
	return httpRequest, nil
}
