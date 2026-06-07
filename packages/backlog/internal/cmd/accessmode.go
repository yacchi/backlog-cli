package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var readOnlySubcommands = map[string]bool{
	"list":     true,
	"view":     true,
	"show":     true,
	"tree":     true,
	"count":    true,
	"download": true,
	"status":   true,
	"current":  true,
	"me":       true,
	"path":     true,
	"logs":     true,
	"get":      true,
	"snapshot": true,
	"myself":   true,
}

var readOnlyTopLevel = map[string]bool{
	"whoami":     true,
	"priority":   true,
	"resolution": true,
	"profile":    true,
	"cli-ref":    true,
}

func checkAccessMode(cmd *cobra.Command) error {
	mode := os.Getenv("BACKLOG_ACCESS_MODE")
	if mode != "read-only" {
		return nil
	}

	name := cmd.Name()

	if cmd.Parent() == nil || cmd.Parent().Name() == "backlog" {
		if readOnlyTopLevel[name] {
			return nil
		}
	}

	if readOnlySubcommands[name] {
		return nil
	}

	if name == "api" {
		method, _ := cmd.Flags().GetString("method")
		if method == "" {
			method = "GET"
		}
		switch strings.ToUpper(method) {
		case "GET", "HEAD", "OPTIONS":
			return nil
		}
	}

	return fmt.Errorf("command '%s' is not allowed in read-only mode", cmd.CommandPath())
}
