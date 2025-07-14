// main.go
package main

import (
	"github.com/subosito/gotenv"
	"github.com/temirov/llm-proxy/cmd"
)

// main is the entry point for gpt-proxy.
func main() {
	_ = gotenv.Load()

	cmd.Execute()
}
