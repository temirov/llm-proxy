// Package main starts the llm-proxy application.
package main

import (
	"fmt"
	"os"

	"github.com/subosito/gotenv"
)

const (
	// environmentLoadErrorMessage is printed when environment variables cannot be loaded.
	environmentLoadErrorMessage = "failed to load environment variables: %v\n"
)

// main is the entry point for llm-proxy.
func main() {
	environmentLoadError := gotenv.Load()
	if environmentLoadError != nil {
		fmt.Fprintf(os.Stderr, environmentLoadErrorMessage, environmentLoadError)
	}

	Execute()
}
