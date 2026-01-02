package main

import (
	"os"

	"github.com/yacchi/backlog-cli/packages/backlog/app"
)

func main() {
	if err := app.Run(); err != nil {
		exitCode := app.HandleError(err)
		os.Exit(int(exitCode))
	}
}
