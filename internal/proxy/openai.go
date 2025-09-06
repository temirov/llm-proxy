package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// openAIRequest sends a prompt to the OpenAI responses API and returns the resulting text.
func openAIRequest(openAIKey string, modelIdentifier string, userPrompt string, systemPrompt string, webSearchEnabled bool, structuredLogger *zap.SugaredLogger) (string, error) {
	messageList := []map[string]string{
		{keyRole: keySystem, keyContent: systemPrompt},
		{keyRole: keyUser, keyContent: userPrompt},
	}

	payload := BuildRequestPayload(modelIdentifier, messageList, webSearchEnabled)
	payloadBytes, marshalError := json.Marshal(payload)
	if marshalError != nil {
		structuredLogger.Errorw(logEventMarshalRequestPayload, constants.LogFieldError, marshalError)
		return constants.EmptyString, errors.New(errorRequestBuild)
	}

	requestContext, cancelRequest := context.WithTimeout(context.Background(), requestTimeout)
	defer cancelRequest()
	httpRequest, buildError := buildAuthorizedJSONRequest(requestContext, http.MethodPost, ResponsesURL(), openAIKey, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return constants.EmptyString, errors.New(errorRequestBuild)
	}

	statusCode, responseBytes, latencyMillis, requestError := performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIRequestError)
	if requestError != nil {
		if errors.Is(requestError, context.DeadlineExceeded) {
			return constants.EmptyString, requestError
		}
		return constants.EmptyString, errors.New(errorOpenAIRequest)
	}

	var decodedObject map[string]any
	if unmarshalError := json.Unmarshal(responseBytes, &decodedObject); unmarshalError != nil {
		structuredLogger.Errorw(logEventParseOpenAIResponseFailed, constants.LogFieldError, unmarshalError)
	}

	outputText := extractTextFromAny(responseBytes)
	responseIdentifier := utils.GetString(decodedObject, jsonFieldID)
	apiStatus := utils.GetString(decodedObject, jsonFieldStatus)

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
		return constants.EmptyString, errors.New(errorOpenAIAPI)
	}

	// If the initial response did not contain text but has an ID, start polling.
	if utils.IsBlank(outputText) && !utils.IsBlank(responseIdentifier) {
		finalText, pollError := pollResponseUntilDone(openAIKey, responseIdentifier, structuredLogger)
		if pollError != nil {
			structuredLogger.Errorw(
				logEventOpenAIPollError,
				logFieldID, responseIdentifier,
				constants.LogFieldError, pollError,
			)
			return constants.EmptyString, errors.New(errorOpenAIAPI)
		}
		if utils.IsBlank(finalText) {
			return constants.EmptyString, errors.New(errorOpenAIAPINoText)
		}
		return finalText, nil
	}

	if utils.IsBlank(outputText) {
		return constants.EmptyString, errors.New(errorOpenAIAPI)
	}
	return outputText, nil
}

// pollResponseUntilDone repeatedly fetches a response until it is complete or the poll timeout elapses.
func pollResponseUntilDone(openAIKey string, responseIdentifier string, structuredLogger *zap.SugaredLogger) (string, error) {
	deadlineInstant := time.Now().Add(upstreamPollTimeout)
	for {
		if time.Now().After(deadlineInstant) {
			return constants.EmptyString, ErrUpstreamIncomplete
		}
		textCandidate, responseComplete, fetchError := fetchResponseByID(deadlineInstant, openAIKey, responseIdentifier, structuredLogger)
		if fetchError != nil {
			return constants.EmptyString, fetchError
		}
		if responseComplete && !utils.IsBlank(textCandidate) {
			return textCandidate, nil
		}
		if responseComplete {
			return constants.EmptyString, errors.New(errorOpenAIAPINoText)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// fetchResponseByID retrieves a response by identifier and reports whether the response is complete.
func fetchResponseByID(deadline time.Time, openAIKey string, responseIdentifier string, structuredLogger *zap.SugaredLogger) (string, bool, error) {
	resourceURL := ResponsesURL() + "/" + responseIdentifier
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	httpRequest, buildError := buildAuthorizedJSONRequest(ctx, http.MethodGet, resourceURL, openAIKey, nil)
	if buildError != nil {
		return constants.EmptyString, false, buildError
	}

	_, responseBytes, _, requestError := performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIPollError)
	if requestError != nil {
		return constants.EmptyString, false, requestError
	}

	var decodedObject map[string]any
	_ = json.Unmarshal(responseBytes, &decodedObject)
	responseStatus := strings.ToLower(utils.GetString(decodedObject, jsonFieldStatus))
	outputText := extractTextFromAny(responseBytes)

	switch responseStatus {
	case statusCompleted, statusSucceeded, statusDone:
		return outputText, true, nil
	case statusCancelled, statusFailed, statusErrored:
		return constants.EmptyString, true, errors.New(errorOpenAIFailedStatus)
	default:
		return constants.EmptyString, false, nil
	}
}

// --- New, Corrected Response Parser ---

type outputItem struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content []contentPart   `json:"content"`
	Action  json.RawMessage `json:"action"`
}
type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type searchAction struct {
	Query string `json:"query"`
}

// joinParts concatenates non-empty texts from content parts.
func joinParts(parts []contentPart) string {
	var builder strings.Builder
	for _, part := range parts {
		if part.Type == "output_text" {
			text := strings.TrimSpace(part.Text)
			if text != constants.EmptyString {
				if builder.Len() > 0 {
					builder.WriteString(constants.LineBreak)
				}
				builder.WriteString(text)
			}
		}
	}
	return builder.String()
}

// extractTextFromAny parses the response according to the new documentation.
func extractTextFromAny(rawPayload []byte) string {
	var envelope struct {
		OutputText string       `json:"output_text"`
		Output     []outputItem `json:"output"`
	}

	if json.Unmarshal(rawPayload, &envelope) != nil {
		return constants.EmptyString
	}

	// 1. Check for the simple `output_text` field first.
	if !utils.IsBlank(envelope.OutputText) {
		return envelope.OutputText
	}

	// 2. If not found, parse the `output` array for a `message` item.
	if len(envelope.Output) > 0 {
		var assistantParts []contentPart
		for _, item := range envelope.Output {
			if item.Type == "message" && item.Role == keyAssistant {
				assistantParts = item.Content
				break
			}
		}
		if text := joinParts(assistantParts); !utils.IsBlank(text) {
			return text
		}
	}

	// 3. As a final fallback, find the last web search query.
	if len(envelope.Output) > 0 {
		lastQuery := ""
		for _, part := range envelope.Output {
			if part.Type == "web_search_call" {
				var action searchAction
				if json.Unmarshal(part.Action, &action) == nil && !utils.IsBlank(action.Query) {
					lastQuery = action.Query
				}
			}
		}
		if !utils.IsBlank(lastQuery) {
			return fmt.Sprintf("Model did not provide a final answer. Last web search: \"%s\"", lastQuery)
		}
	}

	return constants.EmptyString
}

// --- HTTP and Helper Functions ---

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
	retryStrategy := utils.AcquireExponentialBackoff()
	defer utils.ReleaseExponentialBackoff(retryStrategy)
	retryError := backoff.Retry(operation, backoff.WithContext(retryStrategy, httpRequest.Context()))
	return statusCode, responseBytes, latencyMillis, retryError
}

func buildAuthorizedJSONRequest(contextToUse context.Context, method string, resourceURL string, openAIKey string, body io.Reader) (*http.Request, error) {
	httpRequest, httpRequestError := http.NewRequestWithContext(contextToUse, method, resourceURL, body)
	if httpRequestError != nil {
		return nil, httpRequestError
	}
	httpRequest.Header.Set(headerAuthorization, headerAuthorizationPrefix+openAIKey)
	if body != nil {
		httpRequest.Header.Set(headerContentType, mimeApplicationJSON)
	}
	return httpRequest, nil
}
