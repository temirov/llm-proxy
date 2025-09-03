package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testValidator(models ...string) *modelValidator {
	modelSet := make(map[string]struct{}, len(models))
	for _, modelName := range models {
		modelSet[modelName] = struct{}{}
	}
	return &modelValidator{models: modelSet, expiry: time.Now().Add(time.Hour)}
}

func TestValidateConfig(t *testing.T) {
	testCases := []struct {
		config  Configuration
		wantErr string
	}{
		{Configuration{ServiceSecret: "", OpenAIKey: ""}, "SERVICE_SECRET must be set"},
		{Configuration{ServiceSecret: "secret", OpenAIKey: ""}, "OPENAI_API_KEY must be set"},
	}
	for _, testCase := range testCases {
		err := validateConfig(testCase.config)
		if err == nil {
			t.Errorf("validateConfig(%+v) = nil; want error containing %q", testCase.config, testCase.wantErr)
		} else if !strings.Contains(err.Error(), testCase.wantErr) {
			t.Errorf("validateConfig(%+v) error = %q; want contains %q", testCase.config, err.Error(), testCase.wantErr)
		}
	}
}

func TestSecretMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sharedSecret := "s3cr3t"
	router := gin.New()
	router.Use(secretMiddleware(sharedSecret, zap.NewExample().Sugar()))
	router.GET("/", func(context *gin.Context) {
		context.String(http.StatusOK, "OK")
	})

	testCases := []struct {
		key      string
		wantCode int
	}{
		{"", http.StatusForbidden},
		{"wrong", http.StatusForbidden},
		{"s3cr3t", http.StatusOK},
	}
	for _, testCase := range testCases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest("GET", "/?key="+testCase.key, nil)
		router.ServeHTTP(recorder, request)
		if recorder.Code != testCase.wantCode {
			t.Errorf("with key=%q code=%d; want %d", testCase.key, recorder.Code, testCase.wantCode)
		}
	}
}

func TestChatHandler_MissingPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	taskQueue := make(chan requestTask, 1)
	defer close(taskQueue)
	validator := testValidator(defaultModel)
	router.GET("/", chatHandler(taskQueue, "", validator, zap.NewExample().Sugar()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("missing prompt code = %d; want %d", recorder.Code, http.StatusBadRequest)
	}
	if body := recorder.Body.String(); body != "missing prompt parameter" {
		t.Errorf("missing prompt body = %q; want %q", body, "missing prompt parameter")
	}
}

func TestChatHandler_Success(t *testing.T) {
	original := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			const respBody = `{"output_text":"Hello, world!"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respBody)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: 5 * time.Second,
	}
	defer func() { http.DefaultClient = original }()

	gin.SetMode(gin.TestMode)
	taskQueue := make(chan requestTask, 1)
	go func() {
		for pending := range taskQueue {
			text, err := openAIRequest("ignored", pending.model, pending.prompt, pending.systemPrompt, pending.webSearchEnabled, zap.NewExample().Sugar())
			pending.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	validator := testValidator(defaultModel)
	router.GET("/", chatHandler(taskQueue, "", validator, zap.NewExample().Sugar()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?prompt=anything", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("success code = %d; want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.String(); body != "Hello, world!" {
		t.Errorf("success body = %q; want %q", body, "Hello, world!")
	}
}

func TestChatHandler_WithWebSearchFlag_SendsTool(t *testing.T) {
	var captured map[string]any

	original := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			requestBody, _ := io.ReadAll(request.Body)
			_ = json.Unmarshal(requestBody, &captured)
			const respBody = `{"output_text":"search ok"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respBody)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: 5 * time.Second,
	}
	defer func() { http.DefaultClient = original }()

	gin.SetMode(gin.TestMode)
	taskQueue := make(chan requestTask, 1)
	go func() {
		for pending := range taskQueue {
			text, err := openAIRequest("ignored", pending.model, pending.prompt, pending.systemPrompt, pending.webSearchEnabled, zap.NewExample().Sugar())
			pending.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	validator := testValidator(defaultModel)
	router.GET("/", chatHandler(taskQueue, "", validator, zap.NewExample().Sugar()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?prompt=anything&web_search=1", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("web_search code = %d; want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.String(); body != "search ok" {
		t.Errorf("web_search body = %q; want %q", body, "search ok")
	}

	toolsValue, ok := captured["tools"].([]any)
	if !ok || len(toolsValue) == 0 {
		t.Fatalf("tools not present in payload when web_search=1")
	}
	firstTool, _ := toolsValue[0].(map[string]any)
	if firstTool["type"] != "web_search" {
		t.Errorf("tool type = %v; want %q", firstTool["type"], "web_search")
	}
}

func TestChatHandler_CSVFormat(t *testing.T) {
	original := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			const respBody = `{"output_text":"Hello, world!"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respBody)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: 5 * time.Second,
	}
	defer func() { http.DefaultClient = original }()

	gin.SetMode(gin.TestMode)
	taskQueue := make(chan requestTask, 1)
	go func() {
		for pending := range taskQueue {
			text, err := openAIRequest("ignored", pending.model, pending.prompt, pending.systemPrompt, pending.webSearchEnabled, zap.NewExample().Sugar())
			pending.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	validator := testValidator(defaultModel)
	router.GET("/", chatHandler(taskQueue, "", validator, zap.NewExample().Sugar()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?prompt=anything", nil)
	request.Header.Set("Accept", "text/csv")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("csv code = %d; want %d", recorder.Code, http.StatusOK)
	}
	if ct := recorder.Header().Get("Content-Type"); ct != "text/csv" {
		t.Errorf("csv content type = %q; want %q", ct, "text/csv")
	}
	if body := recorder.Body.String(); body != "\"Hello, world!\"\n" {
		t.Errorf("csv body = %q; want %q", body, "\"Hello, world!\"\n")
	}
}

func TestChatHandler_FormatParam(t *testing.T) {
	original := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			const respBody = `{"output_text":"Hello"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respBody)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: 5 * time.Second,
	}
	defer func() { http.DefaultClient = original }()

	gin.SetMode(gin.TestMode)
	taskQueue := make(chan requestTask, 1)
	go func() {
		for pending := range taskQueue {
			text, err := openAIRequest("ignored", pending.model, pending.prompt, pending.systemPrompt, pending.webSearchEnabled, zap.NewExample().Sugar())
			pending.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	validator := testValidator(defaultModel)
	router.GET("/", chatHandler(taskQueue, "", validator, zap.NewExample().Sugar()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?prompt=anything&format=application/json", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("param code = %d; want %d", recorder.Code, http.StatusOK)
	}
	if ct := recorder.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("param content type = %q; want %q", ct, "application/json")
	}
	expected := `{"request":"anything","response":"Hello"}`
	if body := strings.TrimSpace(recorder.Body.String()); body != expected {
		t.Errorf("param body = %q; want %q", body, expected)
	}
}

func TestChatHandler_XMLHeader(t *testing.T) {
	original := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			const respBody = `{"output_text":"Hi"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respBody)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: 5 * time.Second,
	}
	defer func() { http.DefaultClient = original }()

	gin.SetMode(gin.TestMode)
	taskQueue := make(chan requestTask, 1)
	go func() {
		for pending := range taskQueue {
			text, err := openAIRequest("ignored", pending.model, pending.prompt, pending.systemPrompt, pending.webSearchEnabled, zap.NewExample().Sugar())
			pending.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	validator := testValidator(defaultModel)
	router.GET("/", chatHandler(taskQueue, "", validator, zap.NewExample().Sugar()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?prompt=q", nil)
	request.Header.Set("Accept", "application/xml")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("xml code = %d; want %d", recorder.Code, http.StatusOK)
	}
	if ct := recorder.Header().Get("Content-Type"); ct != "application/xml" {
		t.Errorf("xml content type = %q; want %q", ct, "application/xml")
	}
	expected := `<response request="q">Hi</response>`
	if body := strings.TrimSpace(recorder.Body.String()); body != expected {
		t.Errorf("xml body = %q; want %q", body, expected)
	}
}

func TestChatHandler_APIError(t *testing.T) {
	original := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			const respBody = `{"error":{"message":"Bad request"}}`
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(respBody)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: 5 * time.Second,
	}
	defer func() { http.DefaultClient = original }()

	gin.SetMode(gin.TestMode)
	taskQueue := make(chan requestTask, 1)
	defer close(taskQueue)
	go func() {
		for pending := range taskQueue {
			text, err := openAIRequest("ignored", pending.model, pending.prompt, pending.systemPrompt, pending.webSearchEnabled, zap.NewExample().Sugar())
			pending.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	validator := testValidator(defaultModel)
	router.GET("/", chatHandler(taskQueue, "", validator, zap.NewExample().Sugar()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?prompt=test", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Errorf("API error code = %d; want %d", recorder.Code, http.StatusBadGateway)
	}
	if !strings.Contains(recorder.Body.String(), "OpenAI API error") {
		t.Errorf("API error body = %q; want to contain %q", recorder.Body.String(), "OpenAI API error")
	}
}

func TestChatHandler_SystemPromptOverride(t *testing.T) {
	original := http.DefaultClient
	var capturedSystemPrompt string
	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			bodyBytes, _ := io.ReadAll(request.Body)
			var payload map[string]any
			_ = json.Unmarshal(bodyBytes, &payload)
			if inputArray, ok := payload["input"].([]any); ok && len(inputArray) > 0 {
				if systemMap, ok := inputArray[0].(map[string]any); ok {
					if text, ok := systemMap["content"].(string); ok {
						capturedSystemPrompt = text
					}
				}
			}
			const respBody = `{"output_text":"ok"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respBody)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: 5 * time.Second,
	}
	defer func() { http.DefaultClient = original }()

	gin.SetMode(gin.TestMode)
	taskQueue := make(chan requestTask, 1)
	defer close(taskQueue)
	go func() {
		for pending := range taskQueue {
			text, err := openAIRequest("ignored", pending.model, pending.prompt, pending.systemPrompt, pending.webSearchEnabled, zap.NewExample().Sugar())
			pending.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	validator := testValidator(defaultModel)
	router.GET("/", chatHandler(taskQueue, "default", validator, zap.NewExample().Sugar()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?prompt=test&system_prompt=override", nil)
	router.ServeHTTP(recorder, request)

	if capturedSystemPrompt != "override" {
		t.Errorf("system prompt = %q; want %q", capturedSystemPrompt, "override")
	}
}

func TestChatHandler_ModelParam(t *testing.T) {
	original := http.DefaultClient
	var capturedModel string
	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			bodyBytes, _ := io.ReadAll(request.Body)
			var payload map[string]any
			_ = json.Unmarshal(bodyBytes, &payload)
			if modelValue, ok := payload["model"].(string); ok {
				capturedModel = modelValue
			}
			const respBody = `{"output_text":"ok"}`
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(respBody)), Header: make(http.Header)}, nil
		}),
		Timeout: 5 * time.Second,
	}
	defer func() { http.DefaultClient = original }()

	gin.SetMode(gin.TestMode)
	taskQueue := make(chan requestTask, 1)
	defer close(taskQueue)
	go func() {
		for pending := range taskQueue {
			text, err := openAIRequest("ignored", pending.model, pending.prompt, pending.systemPrompt, pending.webSearchEnabled, zap.NewExample().Sugar())
			pending.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	validator := testValidator("custom")
	router.GET("/", chatHandler(taskQueue, "", validator, zap.NewExample().Sugar()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?prompt=test&model=custom", nil)
	router.ServeHTTP(recorder, request)

	if capturedModel != "custom" {
		t.Errorf("model sent = %q; want %q", capturedModel, "custom")
	}
	if recorder.Code != http.StatusOK {
		t.Errorf("model param code = %d; want %d", recorder.Code, http.StatusOK)
	}
}

func TestChatHandler_UnknownModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	taskQueue := make(chan requestTask, 1)
	router := gin.New()
	validator := testValidator("known")
	router.GET("/", chatHandler(taskQueue, "", validator, zap.NewExample().Sugar()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?prompt=hi&model=bad", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("unknown model code = %d; want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "unknown model") {
		t.Errorf("unknown model body = %q; want to contain %q", recorder.Body.String(), "unknown model")
	}
}

func TestChatHandler_Timeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	taskQueue := make(chan requestTask, 1)
	router := gin.New()
	validator := testValidator(defaultModel)
	router.GET("/", chatHandler(taskQueue, "", validator, zap.NewExample().Sugar()))

	originalTimeout := requestTimeout
	requestTimeout = 50 * time.Millisecond
	defer func() { requestTimeout = originalTimeout }()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?prompt=test", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusGatewayTimeout {
		t.Errorf("timeout code = %d; want %d", recorder.Code, http.StatusGatewayTimeout)
	}
	if body := recorder.Body.String(); body != "request timed out" {
		t.Errorf("timeout body = %q; want %q", body, "request timed out")
	}
}

func TestPreferredMime_Normalization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	request := httptest.NewRequest("GET", "/", nil)
	queryValues := request.URL.Query()
	queryValues.Set("format", " Application/JSON ")
	request.URL.RawQuery = queryValues.Encode()
	ctx.Request = request
	if got := preferredMime(ctx); got != "application/json" {
		t.Errorf("preferredMime query = %q; want %q", got, "application/json")
	}

	recorderTwo := httptest.NewRecorder()
	ctxTwo, _ := gin.CreateTestContext(recorderTwo)
	request = httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Accept", "  TeXt/CsV  ")
	ctxTwo.Request = request
	if got := preferredMime(ctxTwo); got != "text/csv" {
		t.Errorf("preferredMime header = %q; want %q", got, "text/csv")
	}
}
