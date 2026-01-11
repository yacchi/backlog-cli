package resolution

import (
	"github.com/spf13/cobra"
)

// ResolutionCmd is the root command for resolution operations
var ResolutionCmd = &cobra.Command{
	Use:   "resolution",
	Short: "Manage resolutions",
	Long:  `List available resolutions for issues.`,
}

func init() {
	ResolutionCmd.AddCommand(listCmd)
}
