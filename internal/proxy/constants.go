package proxy

const (
	// LogLevelDebug indicates that the application should log debug information.
	LogLevelDebug = "debug"

	// LogLevelInfo indicates that the application should log informational messages.
	LogLevelInfo = "info"

	headerAuthorization       = "Authorization"
	headerContentType         = "Content-Type"
	headerAccept              = "Accept"
	headerAuthorizationPrefix = "Bearer "

	queryParameterPrompt       = "prompt"
	queryParameterKey          = "key"
	queryParameterModel        = "model"
	queryParameterWebSearch    = "web_search"
	queryParameterSystemPrompt = "system_prompt"
	queryParameterFormat       = "format"

	redactedPlaceholder = "***REDACTED***"

	mimeApplicationJSON = "application/json"
	mimeApplicationXML  = "application/xml"
	mimeTextXML         = "text/xml"
	mimeTextCSV         = "text/csv"
	mimeTextPlain       = "text/plain; charset=utf-8"

	errorMissingPrompt = "missing prompt parameter"
	// ErrorMissingClientKey indicates that the key query parameter is missing.
	ErrorMissingClientKey   = "missing client key"
	errorRequestTimedOut    = "request timed out"
	errorRequestBuild       = "request build error"
	errorOpenAIRequest      = "OpenAI request error"
	errorOpenAIAPI          = "OpenAI API error"
	errorOpenAIAPINoText    = "OpenAI API error (no text)"
	errorOpenAIFailedStatus = "OpenAI API error (failed status)"
	// errorUpstreamIncomplete indicates that the upstream provider returned an incomplete response.
	errorUpstreamIncomplete    = "OpenAI API error (incomplete response)"
	errorOpenAIModelValidation = "OpenAI model validation error"
	// errorUnknownModel indicates that a model identifier is not recognized.
	errorUnknownModel                = "unknown model"
	errorWebSearchUnsupportedByModel = "web_search is not supported by the selected model"
	errorResponseFormat              = "response formatting error"
	// errorQueueFull indicates that the internal request queue cannot accept additional tasks.
	errorQueueFull = "request queue full"

	toolTypeWebSearch = "web_search"

	keyRole            = "role"
	keyUser            = "user"
	keySystem          = "system"
	keyContent         = "content"
	keyModel           = "model"
	keyInput           = "input"
	keyTemperature     = "temperature"
	keyMaxOutputTokens = "max_output_tokens"
	keyTools           = "tools"
	keyType            = "type"
	keyToolChoice      = "tool_choice"
	keyAuto            = "auto"

	jsonFieldID                   = "id"
	jsonFieldStatus               = "status"
	jsonFieldOutputText           = "output_text"
	jsonFieldResponse             = "response"
	jsonFieldAllowedRequestFields = "allowed_request_fields"

	statusCompleted = "completed"
	statusSucceeded = "succeeded"
	statusDone      = "done"
	statusCancelled = "cancelled"
	statusFailed    = "failed"
	statusErrored   = "errored"

	logFieldHTTPStatus   = "http_status"
	logFieldAPIStatus    = "api_status"
	logFieldResponseText = "response_text"
	// logFieldResponseBody captures the raw body returned by the upstream API.
	logFieldResponseBody = "response_body"
	logFieldMethod       = "method"
	logFieldPath         = "path"
	logFieldClientIP     = "client_ip"
	logFieldStatus       = "status"
	logFieldValue        = "value"
	logFieldError        = "error"
	// logFieldParameter identifies the request parameter related to a log entry.
	logFieldParameter = "parameter"
	// logFieldID identifies the response identifier logged for traceability.
	logFieldID = "id"

	// logFieldExpectedFingerprint identifies the fingerprint of the expected client key.
	logFieldExpectedFingerprint = "expected_fingerprint"

	logEventOpenAIRequestError           = "OpenAI request error"
	logEventOpenAIResponse               = "OpenAI API response"
	logEventOpenAIModelsList             = "OpenAI models list"
	logEventOpenAIModelsListError        = "OpenAI models list error"
	logEventOpenAIModelCapabilitiesError = "OpenAI model capabilities error"
	logEventOpenAIPollError              = "OpenAI poll error"
	// logEventParseOpenAIResponseFailed indicates that parsing the OpenAI response failed.
	logEventParseOpenAIResponseFailed     = "parse OpenAI response failed"
	logEventForbiddenRequest              = "forbidden request"
	logEventRequestReceived               = "request received"
	logEventResponseSent                  = "response sent"
	logEventMarshalRequestPayload         = "marshal request payload failed"
	logEventMarshalResponsePayload        = "marshal response payload failed"
	logEventBuildHTTPRequest              = "build HTTP request failed"
	logEventRetryingWithoutParam          = "retrying without parameter"
	logEventParseWebSearchParameterFailed = "parse web_search parameter failed"

	responseRequestAttribute = "request"
)
