package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/temirov/llm-proxy/internal/logging"
	"github.com/temirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

type HTTPDoer interface {
	Do(httpRequest *http.Request) (*http.Response, error)
}

var (
	HTTPClient          HTTPDoer = http.DefaultClient
	ResponsesURL                 = "https://api.openai.com/v1/responses"
	ModelsURL                    = "https://api.openai.com/v1/models"
	maxOutputTokens              = DefaultMaxOutputTokens
	upstreamPollTimeout          = 10 * time.Second
)

type responsesAPIShim struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// openAIRequest sends a prompt to the OpenAI responses API and returns the resulting text.
// It retries without unsupported parameters and polls for completion when needed.
func openAIRequest(openAIKey string, modelIdentifier string, userPrompt string, systemPrompt string, webSearchEnabled bool, structuredLogger *zap.SugaredLogger) (string, error) {
	messageList := []map[string]string{
		{keyRole: keySystem, keyContent: systemPrompt},
		{keyRole: keyUser, keyContent: userPrompt},
	}

	modelSpecification := resolveModelSpecification(modelIdentifier)

	requestPayload := map[string]any{
		keyModel:           modelIdentifier,
		keyInput:           messageList,
		keyMaxOutputTokens: maxOutputTokens,
	}
	if modelSpecification.IncludeTemperature {
		requestPayload[keyTemperature] = 0.7
	}
	if webSearchEnabled && modelSpecification.IncludeWebSearchTools {
		requestPayload[keyTools] = []any{map[string]any{keyType: toolTypeWebSearch}}
		requestPayload[keyToolChoice] = keyAuto
	}

	payloadBytes, marshalError := json.Marshal(requestPayload)
	if marshalError != nil {
		structuredLogger.Errorw(logEventMarshalRequestPayload, "err", marshalError)
		return "", errors.New(errorRequestBuild)
	}

	httpRequest, buildError := buildAuthorizedJSONRequest(http.MethodPost, ResponsesURL, openAIKey, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, "err", buildError)
		return "", errors.New(errorRequestBuild)
	}

	statusCode, responseBytes, latencyMilliseconds, transportError := performJSONRequest(httpRequest, structuredLogger, logEventOpenAIRequestError)
	if transportError != nil {
		return "", errors.New(errorOpenAIRequest)
	}

	if statusCode >= http.StatusBadRequest &&
		strings.Contains(string(responseBytes), "'temperature'") &&
		requestPayload[keyTemperature] != nil {
		structuredLogger.Infow(logEventRetryingWithoutParam, "parameter", keyTemperature)
		delete(requestPayload, keyTemperature)
		retryPayloadBytes, marshalRetryError := json.Marshal(requestPayload)
		if marshalRetryError != nil {
			structuredLogger.Errorw(logEventMarshalRequestPayload, "err", marshalRetryError)
			return "", errors.New(errorRequestBuild)
		}
		retryRequest, buildRetryError := buildAuthorizedJSONRequest(http.MethodPost, ResponsesURL, openAIKey, bytes.NewReader(retryPayloadBytes))
		if buildRetryError != nil {
			structuredLogger.Errorw(logEventBuildHTTPRequest, "err", buildRetryError)
			return "", errors.New(errorRequestBuild)
		}
		statusCode, responseBytes, latencyMilliseconds, transportError = performJSONRequest(retryRequest, structuredLogger, logEventOpenAIRequestError)
		if transportError != nil {
			return "", errors.New(errorOpenAIRequest)
		}
	}

	if statusCode >= http.StatusBadRequest &&
		strings.Contains(string(responseBytes), "'tools'") &&
		requestPayload[keyTools] != nil {
		structuredLogger.Infow(logEventRetryingWithoutParam, "parameter", keyTools)
		delete(requestPayload, keyTools)
		delete(requestPayload, keyToolChoice)
		retryPayloadBytes, marshalRetryError := json.Marshal(requestPayload)
		if marshalRetryError != nil {
			structuredLogger.Errorw(logEventMarshalRequestPayload, "err", marshalRetryError)
			return "", errors.New(errorRequestBuild)
		}
		retryRequest, buildRetryError := buildAuthorizedJSONRequest(http.MethodPost, ResponsesURL, openAIKey, bytes.NewReader(retryPayloadBytes))
		if buildRetryError != nil {
			structuredLogger.Errorw(logEventBuildHTTPRequest, "err", buildRetryError)
			return "", errors.New(errorRequestBuild)
		}
		statusCode, responseBytes, latencyMilliseconds, transportError = performJSONRequest(retryRequest, structuredLogger, logEventOpenAIRequestError)
		if transportError != nil {
			return "", errors.New(errorOpenAIRequest)
		}
	}

	var decodedObject map[string]any
	if unmarshalError := json.Unmarshal(responseBytes, &decodedObject); unmarshalError != nil {
		decodedObject = nil
	}

	outputText := extractTextFromAny(decodedObject, responseBytes)
	responseIdentifier := getString(decodedObject, jsonFieldID)
	apiStatus := strings.ToLower(getString(decodedObject, jsonFieldStatus))

	structuredLogger.Infow(
		logEventOpenAIResponse,
		logFieldHTTPStatus, statusCode,
		logFieldAPIStatus, apiStatus,
		logging.LogFieldLatencyMilliseconds, latencyMilliseconds,
		logFieldResponseText, outputText,
	)

	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		structuredLogger.Errorw(errorOpenAIAPI, "status", statusCode, "body", string(responseBytes))
		return "", errors.New(errorOpenAIAPI)
	}

	if utils.IsBlank(outputText) && !utils.IsBlank(responseIdentifier) {
		finalText, pollError := pollResponseUntilDone(openAIKey, responseIdentifier, structuredLogger)
		if pollError != nil {
			structuredLogger.Errorw(logEventOpenAIPollError, "id", responseIdentifier, "err", pollError)
			return "", errors.New(errorOpenAIAPI)
		}
		if utils.IsBlank(finalText) {
			structuredLogger.Errorw(errorOpenAIAPI, "status", statusCode, "body", string(responseBytes))
			return "", errors.New(errorOpenAIAPINoText)
		}
		return finalText, nil
	}

	if utils.IsBlank(outputText) {
		structuredLogger.Errorw(errorOpenAIAPI, "status", statusCode, "body", string(responseBytes))
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
		textCandidate, responseComplete, fetchError := fetchResponseByID(openAIKey, responseIdentifier, structuredLogger)
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
func fetchResponseByID(openAIKey string, responseIdentifier string, structuredLogger *zap.SugaredLogger) (string, bool, error) {
	resourceURL := ResponsesURL + "/" + responseIdentifier
	httpRequest, buildError := buildAuthorizedJSONRequest(http.MethodGet, resourceURL, openAIKey, nil)
	if buildError != nil {
		return "", false, buildError
	}

	_, responseBytes, _, transportError := performJSONRequest(httpRequest, structuredLogger, logEventOpenAIPollError)
	if transportError != nil {
		return "", false, errors.New(errorOpenAIRequest)
	}

	var decodedObject map[string]any
	if unmarshalError := json.Unmarshal(responseBytes, &decodedObject); unmarshalError != nil {
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

// extractTextFromAny obtains text content from various possible response shapes.
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
	type outputItem struct {
		Content []contentPart `json:"content"`
	}
	var newShape struct {
		Output []outputItem `json:"output"`
	}
	if unmarshalError := json.Unmarshal(rawPayload, &newShape); unmarshalError == nil && len(newShape.Output) > 0 {
		var textBuilder strings.Builder
		for _, outputEntry := range newShape.Output {
			for _, contentEntry := range outputEntry.Content {
				if !utils.IsBlank(contentEntry.Text) {
					if textBuilder.Len() > 0 {
						textBuilder.WriteString("\n")
					}
					textBuilder.WriteString(contentEntry.Text)
				}
			}
		}
		if textBuilder.Len() > 0 {
			return textBuilder.String()
		}
	}

	var altShape struct {
		Response []struct {
			Content []contentPart `json:"content"`
		} `json:"response"`
	}
	if unmarshalError := json.Unmarshal(rawPayload, &altShape); unmarshalError == nil && len(altShape.Response) > 0 {
		var textBuilder strings.Builder
		for _, responseEntry := range altShape.Response {
			for _, contentEntry := range responseEntry.Content {
				if !utils.IsBlank(contentEntry.Text) {
					if textBuilder.Len() > 0 {
						textBuilder.WriteString("\n")
					}
					textBuilder.WriteString(contentEntry.Text)
				}
			}
		}
		if textBuilder.Len() > 0 {
			return textBuilder.String()
		}
	}

	var legacy responsesAPIShim
	if unmarshalError := json.Unmarshal(rawPayload, &legacy); unmarshalError == nil && len(legacy.Choices) > 0 {
		return legacy.Choices[0].Message.Content
	}
	return ""
}

// buildAuthorizedJSONRequest constructs an HTTP request with authorization and JSON content type headers.
func buildAuthorizedJSONRequest(method string, resourceURL string, openAIKey string, body io.Reader) (*http.Request, error) {
	httpRequest, httpRequestError := http.NewRequest(method, resourceURL, body)
	if httpRequestError != nil {
		return nil, httpRequestError
	}
	httpRequest.Header.Set(headerAuthorization, headerAuthorizationPrefix+openAIKey)
	httpRequest.Header.Set(headerContentType, mimeApplicationJSON)
	return httpRequest, nil
}

// performJSONRequest executes an HTTP request and returns the status code, body, and latency.
// Transport errors are logged using the provided logger.
func performJSONRequest(httpRequest *http.Request, structuredLogger *zap.SugaredLogger, logEventOnTransportError string) (int, []byte, int64, error) {
	startTime := time.Now()
	httpResponse, httpError := HTTPClient.Do(httpRequest)
	latencyMilliseconds := time.Since(startTime).Milliseconds()
	if httpError != nil {
		structuredLogger.Errorw(logEventOnTransportError, "err", httpError, logging.LogFieldLatencyMilliseconds, latencyMilliseconds)
		return 0, nil, latencyMilliseconds, httpError
	}
	defer httpResponse.Body.Close()

	responseBytes, readError := io.ReadAll(httpResponse.Body)
	if readError != nil {
		structuredLogger.Errorw(logging.LogEventReadResponseBodyFailed, "err", readError)
		return httpResponse.StatusCode, nil, latencyMilliseconds, readError
	}
	return httpResponse.StatusCode, responseBytes, latencyMilliseconds, nil
}
