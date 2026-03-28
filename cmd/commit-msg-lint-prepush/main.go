// Package main provides the commit-msg-lint-prepush binary, which runs commit-msg-lint
// explicitly as a git pre-push hook, bypassing the auto-detection in the main binary.
package main

import (
	"fmt"
	"os"

	app "github.com/breml/githooks/internal/hooks/commitmsg"
)

func main() {
	err := app.RunPrePushHook(os.Stdin, os.Args)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
