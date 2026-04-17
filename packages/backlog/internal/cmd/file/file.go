package file

import (
	"github.com/spf13/cobra"
)

var FileCmd = &cobra.Command{
	Use:   "file",
	Short: "Manage project shared files",
	Long:  "Browse and download project shared files.",
}

func init() {
	FileCmd.AddCommand(listCmd)
	FileCmd.AddCommand(downloadCmd)
}
