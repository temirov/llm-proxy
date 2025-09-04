package proxy

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/constants"
	"go.uber.org/zap"
)

// preferredMime determines the response MIME type using the format query parameter or the Accept header.
func preferredMime(ginContext *gin.Context) string {
	if explicitFormat := ginContext.Query(queryParameterFormat); explicitFormat != "" {
		return strings.ToLower(strings.TrimSpace(explicitFormat))
	}
	return strings.ToLower(strings.TrimSpace(ginContext.GetHeader(headerAccept)))
}

// formatResponse renders a textual model output into the requested MIME type and returns the body and content type.
// Encoding failures are logged and result in a plain text error message.
func formatResponse(modelText string, preferred string, originalPrompt string, structuredLogger *zap.SugaredLogger) (string, string) {
	switch {
	case strings.Contains(preferred, mimeApplicationJSON):
		encodedJSON, marshalError := json.Marshal(map[string]string{responseRequestAttribute: originalPrompt, jsonFieldResponse: modelText})
		if marshalError != nil {
			structuredLogger.Errorw(logEventMarshalResponsePayload, constants.LogFieldError, marshalError)
			return errorResponseFormat, mimeTextPlain
		}
		return string(encodedJSON), mimeApplicationJSON
	case strings.Contains(preferred, mimeApplicationXML) || strings.Contains(preferred, mimeTextXML):
		type xmlEnvelope struct {
			XMLName xml.Name `xml:"response"`
			Request string   `xml:"request,attr"`
			Text    string   `xml:",chardata"`
		}
		encodedXML, marshalError := xml.Marshal(xmlEnvelope{Request: originalPrompt, Text: modelText})
		if marshalError != nil {
			structuredLogger.Errorw(logEventMarshalResponsePayload, constants.LogFieldError, marshalError)
			return errorResponseFormat, mimeTextPlain
		}
		return string(encodedXML), mimeApplicationXML
	case strings.Contains(preferred, mimeTextCSV):
		escaped := strings.ReplaceAll(modelText, `"`, `""`)
		return fmt.Sprintf(`"%s"`+"\n", escaped), mimeTextCSV
	default:
		return modelText, mimeTextPlain
	}
}
