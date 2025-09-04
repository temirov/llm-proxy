package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/temirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

type HTTPDoer interface {
	Do(httpRequest *http.Request) (*http.Response, error)
}

var (
	HTTPClient   HTTPDoer = http.DefaultClient
	ResponsesURL string   = "https://api.openai.com/v1/responses"
	ModelsURL    string   = "https://api.openai.com/v1/models"
)

type responsesAPIShim struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func openAIRequest(openAIKey string, modelIdentifier string, userPrompt string, systemPrompt string, webSearchEnabled bool, structuredLogger *zap.SugaredLogger) (string, error) {
	messageList := []map[string]string{
		{keyRole: keySystem, keyContent: systemPrompt},
		{keyRole: keyUser, keyContent: userPrompt},
	}

	// Use model specification to decide what we are allowed to send.
	spec := resolveModelSpecification(modelIdentifier)

	requestPayload := map[string]any{
		keyModel:           modelIdentifier,
		keyInput:           messageList,
		keyMaxOutputTokens: 1024,
	}
	if spec.IncludeTemperature {
		requestPayload[keyTemperature] = 0.7
	}
	if webSearchEnabled && spec.IncludeWebSearchTools {
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

	statusCode, responseBytes, latencyMillis, transportError := performJSONRequest(httpRequest, structuredLogger, logEventOpenAIRequestError)
	if transportError != nil {
		return "", errors.New(errorOpenAIRequest)
	}

	// If upstream rejects 'temperature', retry once without it.
	if statusCode >= http.StatusBadRequest &&
		strings.Contains(string(responseBytes), "'temperature'") &&
		requestPayload[keyTemperature] != nil {
		structuredLogger.Infow(logEventRetryingWithoutParam, "parameter", keyTemperature)
		delete(requestPayload, keyTemperature)
		body, _ := json.Marshal(requestPayload)
		retryReq, _ := buildAuthorizedJSONRequest(http.MethodPost, ResponsesURL, openAIKey, bytes.NewReader(body))
		statusCode, responseBytes, latencyMillis, transportError = performJSONRequest(retryReq, structuredLogger, logEventOpenAIRequestError)
		if transportError != nil {
			return "", errors.New(errorOpenAIRequest)
		}
	}

	// If upstream rejects 'tools', retry once without them.
	if statusCode >= http.StatusBadRequest &&
		strings.Contains(string(responseBytes), "'tools'") &&
		requestPayload[keyTools] != nil {
		structuredLogger.Infow(logEventRetryingWithoutParam, "parameter", keyTools)
		delete(requestPayload, keyTools)
		delete(requestPayload, keyToolChoice)
		body, _ := json.Marshal(requestPayload)
		retryReq, _ := buildAuthorizedJSONRequest(http.MethodPost, ResponsesURL, openAIKey, bytes.NewReader(body))
		statusCode, responseBytes, latencyMillis, transportError = performJSONRequest(retryReq, structuredLogger, logEventOpenAIRequestError)
		if transportError != nil {
			return "", errors.New(errorOpenAIRequest)
		}
	}

	var decodedObject map[string]any
	if err := json.Unmarshal(responseBytes, &decodedObject); err != nil {
		decodedObject = nil
	}

	outputText := extractTextFromAny(decodedObject, responseBytes)
	responseIdentifier := getString(decodedObject, jsonFieldID)
	apiStatus := strings.ToLower(getString(decodedObject, jsonFieldStatus))

	structuredLogger.Infow(
		logEventOpenAIResponse,
		logFieldHTTPStatus, statusCode,
		logFieldAPIStatus, apiStatus,
		logFieldLatencyMs, latencyMillis,
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

func pollResponseUntilDone(openAIKey string, responseIdentifier string, structuredLogger *zap.SugaredLogger) (string, error) {
	deadlineInstant := time.Now().Add(10 * time.Second)
	pollInterval := 300 * time.Millisecond

	for {
		if time.Now().After(deadlineInstant) {
			return "", ErrUpstreamIncomplete
		}
		textCandidate, isDone, fetchError := fetchResponseByID(openAIKey, responseIdentifier, structuredLogger)
		if fetchError != nil {
			return "", fetchError
		}
		if isDone && !utils.IsBlank(textCandidate) {
			return textCandidate, nil
		}
		if isDone {
			return "", ErrUpstreamIncomplete
		}
		time.Sleep(pollInterval)
	}
}

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
	if err := json.Unmarshal(responseBytes, &decodedObject); err != nil {
		decodedObject = nil
	}
	statusValue := strings.ToLower(getString(decodedObject, jsonFieldStatus))
	outputText := extractTextFromAny(decodedObject, responseBytes)

	switch statusValue {
	case statusCompleted, statusSucceeded, statusDone:
		return outputText, true, nil
	case statusCancelled, statusFailed, statusErrored:
		return "", true, errors.New(errorOpenAIFailedStatus)
	default:
		return "", false, nil
	}
}

func getString(container map[string]any, field string) string {
	if container == nil {
		return ""
	}
	if rawValue, present := container[field]; present {
		if castValue, ok := rawValue.(string); ok {
			return castValue
		}
	}
	return ""
}

func extractTextFromAny(container map[string]any, rawPayload []byte) string {
	if container != nil {
		if direct, ok := container[jsonFieldOutputText].(string); ok && !utils.IsBlank(direct) {
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
	if err := json.Unmarshal(rawPayload, &newShape); err == nil && len(newShape.Output) > 0 {
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
	if err := json.Unmarshal(rawPayload, &altShape); err == nil && len(altShape.Response) > 0 {
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
	if err := json.Unmarshal(rawPayload, &legacy); err == nil && len(legacy.Choices) > 0 {
		return legacy.Choices[0].Message.Content
	}
	return ""
}

func buildAuthorizedJSONRequest(method string, url string, openAIKey string, body io.Reader) (*http.Request, error) {
	httpRequest, httpRequestError := http.NewRequest(method, url, body)
	if httpRequestError != nil {
		return nil, httpRequestError
	}
	httpRequest.Header.Set(headerAuthorization, headerAuthorizationPrefix+openAIKey)
	httpRequest.Header.Set(headerContentType, mimeApplicationJSON)
	return httpRequest, nil
}

func performJSONRequest(httpRequest *http.Request, structuredLogger *zap.SugaredLogger, logEventOnTransportError string) (int, []byte, int64, error) {
	// up to 2 retries on transport error or 5xx
	const maxRetries = 2
	var lastCode int
	var lastBody []byte
	var lastLatency int64
	var lastErr error

	// keep a reusable copy of the body if present
	var bodyCopy []byte
	if httpRequest.Body != nil {
		buf, _ := io.ReadAll(httpRequest.Body)
		httpRequest.Body.Close()
		bodyCopy = buf
		httpRequest.Body = io.NopCloser(bytes.NewReader(buf))
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		start := time.Now()
		resp, err := HTTPClient.Do(httpRequest)
		latency := time.Since(start).Milliseconds()
		lastLatency = latency

		if err != nil {
			lastErr = err
			if structuredLogger != nil {
				structuredLogger.Errorw(logEventOnTransportError, "err", err, logFieldLatencyMs, latency, "attempt", attempt)
			}
		} else {
			defer resp.Body.Close()
			b, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				if structuredLogger != nil {
					structuredLogger.Errorw(logEventReadResponseBodyFailed, "err", readErr)
				}
				lastCode, lastBody, lastErr = resp.StatusCode, nil, readErr
			} else {
				// success if <500
				if resp.StatusCode < 500 {
					return resp.StatusCode, b, latency, nil
				}
				lastCode, lastBody, lastErr = resp.StatusCode, b, nil
			}
		}

		// retry only on transport error or 5xx
		if attempt == maxRetries {
			break
		}
		time.Sleep(time.Duration(200*(1<<attempt)) * time.Millisecond)

		// rebuild request body for the next round if needed
		if bodyCopy != nil {
			httpRequest.Body = io.NopCloser(bytes.NewReader(bodyCopy))
		}
	}

	return lastCode, lastBody, lastLatency, lastErr
}
