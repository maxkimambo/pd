package main

import (
	"gcp-disk-migrator/cmd"
	"os"
)

func main() {
	if err := cmd.Execute(); err != nil {
		// Cobra prints errors, but we exit with non-zero status
		os.Exit(1)
	}
}
