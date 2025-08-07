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

// roundTripperFunc lets us stub http.DefaultClient.Do.
type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testValidator(models ...string) *modelValidator {
	m := make(map[string]struct{}, len(models))
	for _, model := range models {
		m[model] = struct{}{}
	}
	return &modelValidator{models: m, expiry: time.Now().Add(time.Hour)}
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
			const respBody = `{"choices":[{"message":{"content":"Hello, world!"}}]}`
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
		for task := range taskQueue {
			text, err := openAIRequest("ignored", task.model, task.prompt, task.systemPrompt, zap.NewExample().Sugar())
			task.reply <- result{text: text, err: err}
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

func TestChatHandler_CSVFormat(t *testing.T) {
	original := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			const respBody = `{"choices":[{"message":{"content":"Hello, world!"}}]}`
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
		for task := range taskQueue {
			text, err := openAIRequest("ignored", task.model, task.prompt, task.systemPrompt, zap.NewExample().Sugar())
			task.reply <- result{text: text, err: err}
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
			const respBody = `{"choices":[{"message":{"content":"Hello"}}]}`
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
		for task := range taskQueue {
			text, err := openAIRequest("ignored", task.model, task.prompt, task.systemPrompt, zap.NewExample().Sugar())
			task.reply <- result{text: text, err: err}
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
			const respBody = `{"choices":[{"message":{"content":"Hi"}}]}`
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
		for task := range taskQueue {
			text, err := openAIRequest("ignored", task.model, task.prompt, task.systemPrompt, zap.NewExample().Sugar())
			task.reply <- result{text: text, err: err}
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
		for task := range taskQueue {
			text, err := openAIRequest("ignored", task.model, task.prompt, task.systemPrompt, zap.NewExample().Sugar())
			task.reply <- result{text: text, err: err}
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
	var got string
	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(request.Body)
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			if msgs, ok := payload["messages"].([]any); ok && len(msgs) > 0 {
				if messageMap, ok := msgs[0].(map[string]any); ok {
					if content, ok := messageMap["content"].(string); ok {
						got = content
					}
				}
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)),
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
		for task := range taskQueue {
			text, err := openAIRequest("ignored", task.model, task.prompt, task.systemPrompt, zap.NewExample().Sugar())
			task.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	validator := testValidator(defaultModel)
	router.GET("/", chatHandler(taskQueue, "default", validator, zap.NewExample().Sugar()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?prompt=test&system_prompt=override", nil)
	router.ServeHTTP(recorder, request)

	if got != "override" {
		t.Errorf("system prompt = %q; want %q", got, "override")
	}
}

func TestChatHandler_ModelParam(t *testing.T) {
	original := http.DefaultClient
	var gotModel string
	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(request.Body)
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			if m, ok := payload["model"].(string); ok {
				gotModel = m
			}
			const respBody = `{"choices":[{"message":{"content":"ok"}}]}`
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(respBody)), Header: make(http.Header)}, nil
		}),
		Timeout: 5 * time.Second,
	}
	defer func() { http.DefaultClient = original }()

	gin.SetMode(gin.TestMode)
	taskQueue := make(chan requestTask, 1)
	defer close(taskQueue)
	go func() {
		for task := range taskQueue {
			text, err := openAIRequest("ignored", task.model, task.prompt, task.systemPrompt, zap.NewExample().Sugar())
			task.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	validator := testValidator("custom")
	router.GET("/", chatHandler(taskQueue, "", validator, zap.NewExample().Sugar()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?prompt=test&model=custom", nil)
	router.ServeHTTP(recorder, request)

	if gotModel != "custom" {
		t.Errorf("model sent = %q; want %q", gotModel, "custom")
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

	// Query parameter takes precedence and should be normalized.
	req := httptest.NewRequest("GET", "/", nil)
	q := req.URL.Query()
	q.Set("format", " Application/JSON ")
	req.URL.RawQuery = q.Encode()
	ctx.Request = req
	if got := preferredMime(ctx); got != "application/json" {
		t.Errorf("preferredMime query = %q; want %q", got, "application/json")
	}

	// Header value should also be normalized.
	recorder2 := httptest.NewRecorder()
	ctx2, _ := gin.CreateTestContext(recorder2)
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "  TeXt/CsV  ")
	ctx2.Request = req
	if got := preferredMime(ctx2); got != "text/csv" {
		t.Errorf("preferredMime header = %q; want %q", got, "text/csv")
	}
}
