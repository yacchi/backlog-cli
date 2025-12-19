package main

import (
	"os"

	"github.com/yacchi/backlog-cli/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		exitCode := cmd.HandleError(err)
		os.Exit(int(exitCode))
	}
}
