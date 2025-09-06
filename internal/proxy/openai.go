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

const (
	synthesisInstructionPrimary = "Now synthesize the final answer with concise citations."
	synthesisInstructionRetry   = "Produce the final answer now as plain text with concise citations. Do not call tools. Do not include hidden reasoning."
)

// UpstreamPollTimeout returns the current upstream poll timeout.
func UpstreamPollTimeout() time.Duration { return upstreamPollTimeout }

// SetUpstreamPollTimeout overrides the upstream poll timeout value.
func SetUpstreamPollTimeout(newTimeout time.Duration) { upstreamPollTimeout = newTimeout }

// hasFinalMessage checks if the response payload contains the terminal assistant message.
func hasFinalMessage(rawPayload []byte) bool {
	var envelope struct {
		Output []json.RawMessage `json:"output"`
	}
	if json.Unmarshal(rawPayload, &envelope) != nil {
		return false // Can't parse, assume not final.
	}
	if len(envelope.Output) == 0 {
		return false // No output, can't be final.
	}

	for _, rawItem := range envelope.Output {
		var header struct {
			Type string `json:"type"`
			Role string `json:"role"`
		}
		if json.Unmarshal(rawItem, &header) == nil && header.Type == responseTypeMessage && header.Role == responseRoleAssistant {
			// Found the message, so this is a truly final response.
			return true
		}
	}

	// No assistant message found.
	return false
}

// openAIRequest sends a prompt to the OpenAI responses API and returns the resulting text.
func openAIRequest(openAIKey string, modelIdentifier string, userPrompt string, systemPrompt string, webSearchEnabled bool, structuredLogger *zap.SugaredLogger) (string, error) {
	// The Responses API expects a single string input. We'll prepend the system prompt to the user prompt.
	var combinedPrompt strings.Builder
	if !utils.IsBlank(systemPrompt) {
		combinedPrompt.WriteString(systemPrompt)
		combinedPrompt.WriteString("\n\n")
	}
	combinedPrompt.WriteString(userPrompt)

	payload := BuildRequestPayload(modelIdentifier, combinedPrompt.String(), webSearchEnabled)
	payloadBytes, marshalError := json.Marshal(payload)
	if marshalError != nil {
		structuredLogger.Errorw(logEventMarshalRequestPayload, constants.LogFieldError, marshalError)
		return constants.EmptyString, marshalError
	}

	requestContext, cancelRequest := context.WithTimeout(context.Background(), requestTimeout)
	defer cancelRequest()
	httpRequest, buildError := buildAuthorizedJSONRequest(requestContext, http.MethodPost, DefaultEndpoints.GetResponsesURL(), openAIKey, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return constants.EmptyString, buildError
	}

	statusCode, responseBytes, latencyMillis, requestError := performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIRequestError)
	if requestError != nil {
		if errors.Is(requestError, context.DeadlineExceeded) {
			return constants.EmptyString, requestError
		}
		return constants.EmptyString, errors.New(errorOpenAIRequest)
	}

	structuredLogger.Debugw(logEventOpenAIInitialResponseBody, logFieldResponseBody, string(responseBytes))

	var decodedObject map[string]any
	_ = json.Unmarshal(responseBytes, &decodedObject)

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

	isTerminalStatus := false
	switch apiStatus {
	case statusCompleted, statusSucceeded, statusDone, statusCancelled, statusFailed, statusErrored:
		isTerminalStatus = true
	}

	// Detect the "completed but no assistant message" edge case.
	forcedSynthesis := false
	if isTerminalStatus && apiStatus == statusCompleted && !hasFinalMessage(responseBytes) {
		// Tool phase finished without a final assistant message.
		forcedSynthesis = true
		structuredLogger.Debugw(logEventMissingFinalMessage)
	}

	// If the state is non-terminal OR we must force a synthesis continuation, proceed accordingly.
	if (!isTerminalStatus || forcedSynthesis) && !utils.IsBlank(responseIdentifier) {

		// Decide which response ID to poll:
		//  - Non-terminal: ask upstream to keep going via POST /{id}/continue, then poll the same id
		//  - Forced synthesis: create a new response (previous_response_id, tool_choice:"none"), then poll the new id
		targetResponseID := responseIdentifier

		if forcedSynthesis {
			newID, synthErr := startSynthesisContinuation(openAIKey, responseIdentifier, modelIdentifier, structuredLogger /*retryOrdinal=*/, 0)
			if synthErr != nil {
				structuredLogger.Errorw(
					logEventOpenAIContinueError,
					logFieldID, responseIdentifier,
					constants.LogFieldError, synthErr,
				)
				return constants.EmptyString, errors.New(errorOpenAIAPI)
			}
			targetResponseID = newID
		} else {
			if continueError := continueResponse(openAIKey, responseIdentifier, structuredLogger); continueError != nil {
				structuredLogger.Errorw(
					logEventOpenAIContinueError,
					logFieldID, responseIdentifier,
					constants.LogFieldError, continueError,
				)
				return constants.EmptyString, errors.New(errorOpenAIAPI)
			}
		}

		finalText, pollError := pollResponseUntilDone(openAIKey, targetResponseID, structuredLogger)
		if pollError != nil {
			structuredLogger.Errorw(
				logEventOpenAIPollError,
				logFieldID, targetResponseID,
				constants.LogFieldError, pollError,
			)
			return constants.EmptyString, errors.New(errorOpenAIAPI)
		}
		if !utils.IsBlank(finalText) {
			return finalText, nil
		}

		// --- Fallback: one more synthesis continuation if still no text ---
		if forcedSynthesis {
			structuredLogger.Debugw(logEventRetryingSynthesis)
			newID, synthErr := startSynthesisContinuation(openAIKey, targetResponseID, modelIdentifier, structuredLogger /*retryOrdinal=*/, 1)
			if synthErr != nil {
				structuredLogger.Errorw(
					logEventOpenAIContinueError,
					logFieldID, targetResponseID,
					constants.LogFieldError, synthErr,
				)
				return constants.EmptyString, errors.New(errorOpenAIAPI)
			}
			targetResponseID = newID

			finalText2, pollError2 := pollResponseUntilDone(openAIKey, targetResponseID, structuredLogger)
			if pollError2 != nil {
				structuredLogger.Errorw(
					logEventOpenAIPollError,
					logFieldID, targetResponseID,
					constants.LogFieldError, pollError2,
				)
				return constants.EmptyString, errors.New(errorOpenAIAPI)
			}
			if !utils.IsBlank(finalText2) {
				return finalText2, nil
			}
		}

		return constants.EmptyString, errors.New(errorOpenAIAPINoText)
	}

	// If the initial response is terminal but we couldn't extract text, it's an error.
	if utils.IsBlank(outputText) {
		return constants.EmptyString, errors.New(errorOpenAIAPI)
	}
	return outputText, nil
}

// continueResponse signals to the API that a response session should proceed (legacy non-terminal case).
func continueResponse(openAIKey string, responseIdentifier string, structuredLogger *zap.SugaredLogger) error {
	resourceURL := DefaultEndpoints.GetResponsesURL() + "/" + responseIdentifier + "/continue"
	requestContext, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	httpRequest, buildError := buildAuthorizedJSONRequest(requestContext, http.MethodPost, resourceURL, openAIKey, nil)
	if buildError != nil {
		return buildError
	}

	statusCode, responseBytes, _, requestError := performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIContinueError)
	if requestError != nil {
		return requestError
	}

	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		structuredLogger.Desugar().Error(
			errorOpenAIContinue,
			zap.Int(logFieldStatus, statusCode),
			zap.ByteString(logFieldResponseBody, responseBytes),
			zap.String(logFieldID, responseIdentifier),
		)
		return errors.New(errorOpenAIContinue)
	}
	return nil
}

// startSynthesisContinuation begins a synthesis-only pass by POSTing /v1/responses with
// previous_response_id and tool_choice set to "none". It allocates enough output tokens,
// limits reasoning effort to minimal, and includes a low-verbosity text format hint.
// When retryOrdinal is 1 the instruction is strengthened and the token limit is increased.
// It returns the identifier of the new response.
//
// retryOrdinal==0 : first synthesis pass; retryOrdinal==1 : stricter retry
func startSynthesisContinuation(openAIKey string, previousResponseID string, modelIdentifier string, structuredLogger *zap.SugaredLogger, retryOrdinal int) (string, error) {
	outputTokenLimit := maxOutputTokens
	if outputTokenLimit < 1536 {
		outputTokenLimit = 1536
	}
	if retryOrdinal == 1 {
		if outputTokenLimit < 2048 {
			outputTokenLimit = 2048
		}
	}

	instruction := synthesisInstructionPrimary
	if retryOrdinal == 1 {
		instruction = synthesisInstructionRetry
	}

	payload := map[string]any{
		keyModel:              modelIdentifier,
		keyPreviousResponseID: previousResponseID,
		keyToolChoice:         toolChoiceNone,
		keyInput:              instruction,
		keyMaxOutputTokens:    outputTokenLimit,
		keyReasoning: map[string]any{
			keyEffort: reasoningEffortMinimal,
		},
		keyText: map[string]any{
			keyFormat:    map[string]any{keyType: textFormatType},
			keyVerbosity: verbosityLow,
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	requestContext, cancelRequest := context.WithTimeout(context.Background(), requestTimeout)
	defer cancelRequest()
	request, buildError := buildAuthorizedJSONRequest(requestContext, http.MethodPost, DefaultEndpoints.GetResponsesURL(), openAIKey, bytes.NewReader(payloadBytes))
	if buildError != nil {
		return constants.EmptyString, buildError
	}

	statusCode, responseBytes, _, requestError := performResponsesRequest(request, structuredLogger, logEventOpenAIRequestError)
	if requestError != nil {
		return constants.EmptyString, requestError
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return constants.EmptyString, errors.New(errorOpenAIAPI)
	}

	var decodedResponse map[string]any
	if json.Unmarshal(responseBytes, &decodedResponse) != nil {
		return constants.EmptyString, errors.New(errorOpenAIAPI)
	}
	newID := utils.GetString(decodedResponse, jsonFieldID)
	if utils.IsBlank(newID) {
		return constants.EmptyString, errors.New(errorOpenAIAPI)
	}
	return newID, nil
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
	resourceURL := DefaultEndpoints.GetResponsesURL() + "/" + responseIdentifier
	requestContext, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	httpRequest, buildError := buildAuthorizedJSONRequest(requestContext, http.MethodGet, resourceURL, openAIKey, nil)
	if buildError != nil {
		return constants.EmptyString, false, buildError
	}

	_, responseBytes, _, requestError := performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIPollError)
	if requestError != nil {
		return constants.EmptyString, false, requestError
	}

	structuredLogger.Debugw(
		"OpenAI poll response body",
		logFieldID, responseIdentifier,
		logFieldResponseBody, string(responseBytes),
	)

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

// --- Final, Corrected Response Parser ---
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

// joinParts creates a single string by joining the trimmed text from each
// provided content part using a line break when multiple parts contain text.
func joinParts(parts []contentPart) string {
	var builder strings.Builder
	for _, part := range parts {
		if part.Type == outputPartType || part.Type == textPartType {
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

// extractTextFromAny parses the final response from OpenAI.
func extractTextFromAny(rawPayload []byte) string {
	var envelope struct {
		OutputText string            `json:"output_text"`
		Output     []json.RawMessage `json:"output"` // Use json.RawMessage for resilience
	}

	if json.Unmarshal(rawPayload, &envelope) != nil {
		return constants.EmptyString
	}

	// 1. Prioritize `output_text` as the most reliable source.
	if !utils.IsBlank(envelope.OutputText) {
		return envelope.OutputText
	}

	// 2. If `output_text` is missing, parse the `output` array for the assistant's message.
	if len(envelope.Output) > 0 {
		for _, rawItem := range envelope.Output {
			var header struct {
				Type string `json:"type"`
				Role string `json:"role"`
			}
			if json.Unmarshal(rawItem, &header) == nil && header.Type == responseTypeMessage && header.Role == responseRoleAssistant {
				var msgItem outputItem
				if json.Unmarshal(rawItem, &msgItem) == nil {
					return joinParts(msgItem.Content)
				}
			}
		}
	}

	// 3. If no message was found, create a fallback from the last tool call.
	if len(envelope.Output) > 0 {
		lastQuery := constants.EmptyString
		for outputIndex := len(envelope.Output) - 1; outputIndex >= 0; outputIndex-- {
			rawItem := envelope.Output[outputIndex]
			var header struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(rawItem, &header) == nil && header.Type == responseTypeWebSearchCall {
				var searchItem struct {
					Action searchAction `json:"action"`
				}
				if json.Unmarshal(rawItem, &searchItem) == nil && !utils.IsBlank(searchItem.Action.Query) {
					lastQuery = searchItem.Action.Query
					break
				}
			}
		}
		if !utils.IsBlank(lastQuery) {
			return fmt.Sprintf(fallbackFinalAnswerFormat, lastQuery)
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
		// Retry on server errors (5xx) and rate limit errors (429).
		if statusCode >= http.StatusInternalServerError || statusCode == http.StatusTooManyRequests {
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
	httpReq, httpRequestError := http.NewRequestWithContext(contextToUse, method, resourceURL, body)
	if httpRequestError != nil {
		return nil, httpRequestError
	}
	httpReq.Header.Set(headerAuthorization, headerAuthorizationPrefix+openAIKey)
	if body != nil {
		httpReq.Header.Set(headerContentType, mimeApplicationJSON)
	}
	return httpReq, nil
}
