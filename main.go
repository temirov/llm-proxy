// Package main starts the llm-proxy application.
package main

import (
	"github.com/subosito/gotenv"
	"github.com/temirov/llm-proxy/cmd"
)

// main is the entry point for llm-proxy.
func main() {
	_ = gotenv.Load()

	cmd.Execute()
}
