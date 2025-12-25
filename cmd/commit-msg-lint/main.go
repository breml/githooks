// Package main provides the commit-msg-lint CLI tool for validating commit messages.
package main

import (
	"fmt"
	"os"

	app "github.com/breml/githooks/internal/hooks/commitmsg"
)

func main() {
	err := app.Run(os.Stdin, os.Args)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
