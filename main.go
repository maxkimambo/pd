package main

import (
	"os"

	"github.com/maxkimambo/pd/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
