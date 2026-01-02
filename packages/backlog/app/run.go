package app

import "github.com/yacchi/backlog-cli/packages/backlog/internal/cmd"

// Run executes the CLI application.
func Run() error {
	return cmd.Execute()
}

// HandleError returns an exit code for the given error.
func HandleError(err error) cmd.ExitCode {
	return cmd.HandleError(err)
}
