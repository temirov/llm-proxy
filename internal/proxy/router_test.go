package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Test string constants.
const (
	promptValue             = "hello"
	modelIdentifierValue    = "gpt-4o"
	systemPromptValue       = "system"
	requestQueryFormat      = "/?prompt=%s&model=%s"
	messageUnexpectedStatus = "status=%d want=%d"
)

// TestChatHandlerReturnsBadRequestForUnknownModel verifies that chatHandler returns HTTP 400 when the model is unknown.
func TestChatHandlerReturnsBadRequestForUnknownModel(testFramework *testing.T) {
	validator := &modelValidator{
		models: map[string]struct{}{modelIdentifierValue: {}},
		expiry: time.Now().Add(time.Hour),
	}

	taskQueue := make(chan requestTask, 1)
	go func() {
		pendingTask := <-taskQueue
		pendingTask.reply <- result{err: fmt.Errorf("%w: %s", ErrUnknownModel, pendingTask.model)}
	}()

	loggerInstance, _ := zap.NewDevelopment()
	defer loggerInstance.Sync()

	handler := chatHandler(taskQueue, systemPromptValue, validator, loggerInstance.Sugar())

	responseRecorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(responseRecorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf(requestQueryFormat, promptValue, modelIdentifierValue), nil)

	handler(ginContext)

	if responseRecorder.Code != http.StatusBadRequest {
		testFramework.Fatalf(messageUnexpectedStatus, responseRecorder.Code, http.StatusBadRequest)
	}
}
