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
	router.GET("/", chatHandler(taskQueue, "", zap.NewExample().Sugar()))

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
			text, err := openAIRequest("ignored", task.prompt, task.systemPrompt, zap.NewExample().Sugar())
			task.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	router.GET("/", chatHandler(taskQueue, "", zap.NewExample().Sugar()))

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
			text, err := openAIRequest("ignored", task.prompt, task.systemPrompt, zap.NewExample().Sugar())
			task.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	router.GET("/", chatHandler(taskQueue, "", zap.NewExample().Sugar()))

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
			text, err := openAIRequest("ignored", task.prompt, task.systemPrompt, zap.NewExample().Sugar())
			task.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	router.GET("/", chatHandler(taskQueue, "", zap.NewExample().Sugar()))

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
			text, err := openAIRequest("ignored", task.prompt, task.systemPrompt, zap.NewExample().Sugar())
			task.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	router.GET("/", chatHandler(taskQueue, "", zap.NewExample().Sugar()))

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
			text, err := openAIRequest("ignored", task.prompt, task.systemPrompt, zap.NewExample().Sugar())
			task.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	router.GET("/", chatHandler(taskQueue, "", zap.NewExample().Sugar()))

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
				if m, ok := msgs[0].(map[string]any); ok {
					if c, ok := m["content"].(string); ok {
						got = c
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
			text, err := openAIRequest("ignored", task.prompt, task.systemPrompt, zap.NewExample().Sugar())
			task.reply <- result{text: text, err: err}
		}
	}()
	router := gin.New()
	router.GET("/", chatHandler(taskQueue, "default", zap.NewExample().Sugar()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?prompt=test&system_prompt=override", nil)
	router.ServeHTTP(recorder, request)

	if got != "override" {
		t.Errorf("system prompt = %q; want %q", got, "override")
	}
}

func TestChatHandler_Timeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	taskQueue := make(chan requestTask, 1)
	router := gin.New()
	router.GET("/", chatHandler(taskQueue, "", zap.NewExample().Sugar()))

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
