package proxy

import (
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// ErrUnknownModel is returned when a model identifier is not recognized.
var ErrUnknownModel = errors.New(errorUnknownModel)

// modelValidator validates model identifiers using the static payload schema table.
type modelValidator struct{}

// newModelValidator creates a modelValidator. The parameters are retained for signature compatibility.
func newModelValidator(openAIKey string, structuredLogger *zap.SugaredLogger) (*modelValidator, error) {
	_ = openAIKey
	_ = structuredLogger
	return &modelValidator{}, nil
}

// Verify checks whether the provided model identifier is known after normalization.
func (validator *modelValidator) Verify(modelIdentifier string) error {
	normalized := strings.ToLower(strings.TrimSpace(modelIdentifier))
	if _, known := modelPayloadSchemas[normalized]; !known {
		return fmt.Errorf("%w: %s", ErrUnknownModel, modelIdentifier)
	}
	return nil
}
