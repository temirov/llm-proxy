package tests_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestIntegration_WebSearch_UnsupportedModel_Returns400(t *testing.T) {
	openAISrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/models"):
			io.WriteString(w, `{"data":[{"id":"gpt-4o-mini"},{"id":"gpt-4o"}]}`)
		case strings.HasSuffix(r.URL.Path, "/v1/responses"):
			io.WriteString(w, `{"output_text":"SHOULD_NOT_BE_CALLED"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer openAISrv.Close()

	proxy.SetModelsURL(openAISrv.URL + "/v1/models")
	proxy.SetResponsesURL(openAISrv.URL + "/v1/responses")
	proxy.HTTPClient = openAISrv.Client()
	t.Cleanup(proxy.ResetModelsURL)
	t.Cleanup(proxy.ResetResponsesURL)

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     4,
	}, logger.Sugar())
	if err != nil {
		t.Fatalf("BuildRouter error: %v", err)
	}

	app := httptest.NewServer(router)
	defer app.Close()

	req, _ := http.NewRequest("GET", app.URL+"/?prompt=x&key=sekret&model=gpt-4o-mini&web_search=1", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d", res.StatusCode, http.StatusBadRequest)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "web_search is not supported") {
		t.Fatalf("body=%q missing capability message", string(body))
	}
}

func TestIntegration_TemperatureUnsupportedModel_RetriesWithoutTemperature(t *testing.T) {
	var observed any

	openAISrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/models"):
			io.WriteString(w, `{"data":[{"id":"gpt-5-mini"}]}`)
		case strings.HasSuffix(r.URL.Path, "/v1/responses"):
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &observed)
			if strings.Contains(string(body), `"temperature"`) {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, `{"error":{"message":"Unsupported parameter: 'temperature'"}}`)
				return
			}
			io.WriteString(w, `{"output_text":"TEMPLESS_OK"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer openAISrv.Close()

	proxy.SetModelsURL(openAISrv.URL + "/v1/models")
	proxy.SetResponsesURL(openAISrv.URL + "/v1/responses")
	proxy.HTTPClient = openAISrv.Client()
	t.Cleanup(proxy.ResetModelsURL)
	t.Cleanup(proxy.ResetResponsesURL)

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	router, err := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret: "sekret",
		OpenAIKey:     "sk-test",
		LogLevel:      "debug",
		WorkerCount:   1,
		QueueSize:     4,
	}, logger.Sugar())
	if err != nil {
		t.Fatalf("BuildRouter error: %v", err)
	}

	app := httptest.NewServer(router)
	defer app.Close()

	res, err := http.Get(app.URL + "/?prompt=hello&key=sekret&model=gpt-5-mini")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d body=%s", res.StatusCode, string(b))
	}
	b, _ := io.ReadAll(res.Body)
	if strings.TrimSpace(string(b)) != "TEMPLESS_OK" {
		t.Fatalf("body=%q want %q", string(b), "TEMPLESS_OK")
	}
}
