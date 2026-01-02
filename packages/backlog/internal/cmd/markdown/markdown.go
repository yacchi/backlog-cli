package markdown

import "github.com/spf13/cobra"

// MarkdownCmd groups markdown-related subcommands.
var MarkdownCmd = &cobra.Command{
	Use:   "markdown",
	Short: "Markdown utilities",
	Long:  "View and manage markdown conversion logs.",
}

func init() {
	MarkdownCmd.AddCommand(logsCmd)
}
