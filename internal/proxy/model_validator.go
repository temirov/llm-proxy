package proxy

import (
	"errors"
	"fmt"

	"go.uber.org/zap"
)

// errUnknownModelFormat specifies the format string for wrapping an unknown model error.
const errUnknownModelFormat = "%w: %s"

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

// Verify checks whether the provided model identifier is known.
func (validator *modelValidator) Verify(modelIdentifier string) error {
	if _, known := modelPayloadSchemas[modelIdentifier]; !known {
		return fmt.Errorf(errUnknownModelFormat, ErrUnknownModel, modelIdentifier)
	}
	return nil
}
