package apperrors

import "errors"

var (
	ErrMissingServiceSecret = errors.New("SERVICE_SECRET must be set")
	ErrMissingOpenAIKey     = errors.New("OPENAI_API_KEY must be set")
)
