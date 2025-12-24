package main

import (
	"fmt"
	"os"

	app "github.com/breml/githooks/internal/hooks/commitmsg"
)

func main() {
	err := app.Run(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
