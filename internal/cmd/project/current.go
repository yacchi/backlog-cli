package project

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/config"
)

var currentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show current project key",
	Long: `Show the currently configured project key.

The project key is determined from (in order of priority):
  1. -p/--project flag
  2. .backlog.yaml in current or parent directory
  3. Profile configuration (client.default.project)

Examples:
  backlog project current
  backlog project current --quiet`,
	RunE: runCurrent,
}

var currentQuiet bool

func init() {
	currentCmd.Flags().BoolVarP(&currentQuiet, "quiet", "q", false, "Exit with code 0 if project is set, 1 otherwise (no output)")
}

func runCurrent(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		if currentQuiet {
			os.Exit(1)
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	projectKey := cmdutil.GetCurrentProject(cfg)

	if currentQuiet {
		if projectKey == "" {
			os.Exit(1)
		}
		return nil
	}

	if projectKey == "" {
		fmt.Fprintln(os.Stderr, "No project configured")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Set project with:")
		fmt.Fprintln(os.Stderr, "  backlog config set client.default.project <key>")
		fmt.Fprintln(os.Stderr, "  or create .backlog.yaml in repository root")
		os.Exit(1)
	}

	fmt.Println(projectKey)
	return nil
}
