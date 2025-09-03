// Package main starts the llm-proxy application.
package main

import (
	"github.com/subosito/gotenv"
)

// main is the entry point for llm-proxy.
func main() {
	_ = gotenv.Load()

	Execute()
}
