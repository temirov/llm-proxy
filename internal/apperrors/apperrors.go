// Package apperrors provides shared application error values.
package apperrors

import "errors"

const (
	environmentVariableServiceSecret = "SERVICE_SECRET"
	environmentVariableOpenAIAPIKey  = "OPENAI_API_KEY"
	messageSuffixMustBeSet           = " must be set"
	messageMissingServiceSecret      = environmentVariableServiceSecret + messageSuffixMustBeSet
	messageMissingOpenAIKey          = environmentVariableOpenAIAPIKey + messageSuffixMustBeSet
)

var (
	// ErrMissingServiceSecret is returned when the SERVICE_SECRET environment variable is not defined.
	ErrMissingServiceSecret = errors.New(messageMissingServiceSecret)
	// ErrMissingOpenAIKey is returned when the OPENAI_API_KEY environment variable is not defined.
	ErrMissingOpenAIKey = errors.New(messageMissingOpenAIKey)
)
