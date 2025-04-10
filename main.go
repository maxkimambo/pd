package main

import (
	"os"

	"github.com/maxkimambo/pd/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		// Cobra prints errors, but we exit with non-zero status
		os.Exit(1)
	}
}
