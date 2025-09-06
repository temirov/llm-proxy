package main

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/temirov/llm-proxy/internal/utils"
)

// populateStringConfiguration resolves a string value from command flags, environment variables and defaults.
// flagName specifies the CLI flag, configurationKey maps to the viper key, destination receives the result,
// defaultValue is applied when no value is provided, and transformer adjusts the retrieved value before assignment.
func populateStringConfiguration(command *cobra.Command, flagName, configurationKey string, destination *string, defaultValue string, transformer func(string) string) {
	if !command.Flags().Changed(flagName) {
		*destination = transformer(viper.GetString(configurationKey))
	}
	if utils.IsBlank(*destination) {
		*destination = defaultValue
	}
}

// populateIntConfiguration resolves an integer value from command flags, environment variables and defaults.
// flagName specifies the CLI flag, configurationKey maps to the viper key, destination receives the result,
// and defaultValue replaces non-positive values.
func populateIntConfiguration(command *cobra.Command, flagName, configurationKey string, destination *int, defaultValue int) {
	if !command.Flags().Changed(flagName) {
		*destination = viper.GetInt(configurationKey)
	}
	if *destination <= 0 {
		*destination = defaultValue
	}
}

// identityTransformer returns the supplied value unchanged.
func identityTransformer(value string) string {
	return value
}

// trimSpacesAndQuotes removes surrounding whitespace and quote characters.
func trimSpacesAndQuotes(value string) string {
	return strings.TrimSpace(strings.Trim(value, quoteCharacters))
}
